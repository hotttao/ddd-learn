package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	// Kitex client，用于创建 ADS gRPC 客户端。
	"github.com/cloudwego/kitex/client"
	// Envoy 原生 proto 定义 Node，用于描述 xDS 客户端身份。
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	// xDS ADS 的 DiscoveryRequest / DiscoveryResponse 定义。
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	// Kitex xDS 自动生成的 ADS 客户端，不是原生 grpc-go 客户端。
	"github.com/kitex-contrib/xds/core/api/kitex_gen/envoy/service/discovery/v3/aggregateddiscoveryservice"
	// Kitex xDS 的资源 TypeURL 常量和各类资源解码方法。
	"github.com/kitex-contrib/xds/core/xdsresource"
	// 用于构造 xDS Node.Metadata。
	"google.golang.org/protobuf/types/known/structpb"
)

// istiod 的集群内明文 xDS 地址，仅用于本步骤的兼容性探针。
const defaultIstiodAddress = "istiod.istio-system.svc:15010"

const (
	defaultTargetService = "xhs-service.ddd-learn-sidecar.svc.cluster.local"
	defaultTargetPort    = 80
)

// adsStream 是 Kitex ADS 双向流需要的最小接口。
// 将流抽象为接口后，每个 getXDS 方法都可以专注于一种 Discovery Service。
type adsStream interface {
	Send(*discoveryv3.DiscoveryRequest) error
	Recv() (*discoveryv3.DiscoveryResponse, error)
}

// compatibilityGate 保存一次兼容性检查所需的全部状态。
// endpointNames 保存 CDS -> EDS 的依赖名称；
// counts 保存每类资源收到的数量，避免在 main 中传递多个中间结果。
type compatibilityGate struct {
	stream adsStream
	node   *corev3.Node

	endpointNames []string
	counts        map[string]int
	subscriptions map[string][]string

	targetService      string
	targetClusterNames []string
}

func main() {
	// 设置全局超时，防止 istiod 不可达或协议不兼容时探针永久等待。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 这些值由 Kubernetes Downward API 注入，用于构造 Istio 能识别的 xDS Node。
	podIP := requiredEnv("INSTANCE_IP")
	podName := requiredEnv("POD_NAME")
	namespace := requiredEnv("POD_NAMESPACE")
	istiodAddress := getenv("KITEX_XDS_ISTIO_ADDR", defaultIstiodAddress)
	targetService := getenv("TARGET_SERVICE", defaultTargetService)
	targetPort, err := strconv.Atoi(getenv("TARGET_PORT", strconv.Itoa(defaultTargetPort)))
	if err != nil || targetPort <= 0 {
		log.Fatalf("invalid TARGET_PORT: %q", getenv("TARGET_PORT", strconv.Itoa(defaultTargetPort)))
	}
	targetClusterNames := buildClusterNames(targetService, targetPort, getenv("TARGET_SUBSETS", ""))

	// Node.Metadata 描述客户端所处的运行环境。istiod 会据此计算当前 Pod
	// 在所在 namespace 中应该看到的服务发现和流量配置。
	metadata, err := structpb.NewStruct(map[string]any{
		"CLUSTER_ID":        "Kubernetes",
		"CONFIG_NAMESPACE":  namespace,
		"DNS_AUTO_ALLOCATE": "true",
		"DNS_CAPTURE":       "true",
		"INSTANCE_IPS":      podIP,
		"INTERCEPTION_MODE": "NONE",
		"NAMESPACE":         namespace,
	})
	if err != nil {
		log.Fatalf("build node metadata: %v", err)
	}

	// Node ID 沿用 Istio sidecar 的标准格式：
	// sidecar~PodIP~PodName.Namespace~Namespace.svc.cluster.local。
	// 探针不是 sidecar，但使用该格式可以让 istiod 按普通工作负载视角下发配置。
	node := &corev3.Node{
		Id:       fmt.Sprintf("sidecar~%s~%s.%s~%s.svc.cluster.local", podIP, podName, namespace, namespace),
		Cluster:  "kitex-xds-compatibility",
		Metadata: metadata,
	}

	// 使用 kitex-contrib/xds 生成的 Kitex ADS 客户端，验证业务代码使用的
	// Kitex xDS 依赖能否与当前 Istio/istiod 通信，而不是只验证通用 gRPC。
	adsClient, err := aggregateddiscoveryservice.NewClient(
		"istiod.istio-system.svc",
		client.WithHostPorts(istiodAddress),
	)
	if err != nil {
		log.Fatalf("create Kitex ADS client: %v", err)
	}

	// ADS（Aggregated Discovery Service）使用一条双向流传输多种 xDS 资源。
	stream, err := adsClient.StreamAggregatedResources(ctx)
	if err != nil {
		log.Fatalf("open ADS stream to %s: %v", istiodAddress, err)
	}

	// main 只负责组装兼容性检查对象并启动流程；资源获取和状态维护由结构体完成。
	gate := &compatibilityGate{
		stream:             stream,
		node:               node,
		counts:             make(map[string]int, 5),
		subscriptions:      make(map[string][]string, 5),
		targetService:      targetService,
		targetClusterNames: targetClusterNames,
	}
	if err := gate.run(); err != nil {
		log.Fatal(err)
	}
}

// run 按 NDS -> CDS -> EDS 的 Proxyless outbound 依赖顺序调用各类 Discovery Service 方法。
func (g *compatibilityGate) run() error {
	if err := g.getNDS(); err != nil {
		return err
	}
	if err := g.getCDS(); err != nil {
		return err
	}
	if err := g.getEDS(); err != nil {
		return err
	}

	// NDS、CDS、EDS 都成功解析并 ACK，说明当前 Proxyless outbound 路径通过兼容性门禁。
	names := make([]string, 0, len(g.counts))
	for typeURL, count := range g.counts {
		names = append(names, fmt.Sprintf("%s=%d", shortType(typeURL), count))
	}
	sort.Strings(names)
	log.Printf("compatibility gate passed: %v", names)
	return nil
}

// getNDS 获取 NameTable（NDS）。NDS 为 Proxyless 客户端提供服务名解析信息。
func (g *compatibilityGate) getNDS() error {
	response, err := g.requestResource(xdsresource.NameTableTypeURL, nil)
	if err != nil {
		return err
	}
	resources, err := xdsresource.UnmarshalNDS(response.GetResources())
	if err != nil {
		return fmt.Errorf("decode NDS: %w", err)
	}
	ips, found := resources.NameTable[g.targetService]
	if !found {
		return fmt.Errorf("NDS does not contain target service %q", g.targetService)
	}
	log.Printf("NDS target service=%s ips=%v", g.targetService, ips)
	if err := g.acknowledge(response, nil); err != nil {
		return err
	}
	g.counts[xdsresource.NameTableTypeURL] = 1
	return nil
}

// getCDS 获取 Cluster（CDS），并提取 EDS 类型集群引用的 endpoint 名称。
// 提取出的名称保存到结构体，会作为 getED
// S 的精确 ResourceNames。
func (g *compatibilityGate) getCDS() error {
	response, err := g.requestResource(xdsresource.ClusterTypeURL, g.targetClusterNames)
	if err != nil {
		return err
	}
	resources, err := xdsresource.UnmarshalCDS(response.GetResources())
	if err != nil {
		return fmt.Errorf("decode CDS: %w", err)
	}
	targetResources := filterResources(resources, g.targetClusterNames)
	log.Printf("CDS target resource names=%v", sortedNames(targetResources))

	for _, resource := range targetResources {
		cluster, ok := resource.(*xdsresource.ClusterResource)
		if ok && cluster.DiscoveryType == xdsresource.ClusterDiscoveryTypeEDS && cluster.EndpointName != "" {
			g.endpointNames = appendUnique(g.endpointNames, cluster.EndpointName)
		}
	}
	sort.Strings(g.endpointNames)
	if err := g.acknowledge(response, g.targetClusterNames); err != nil {
		return err
	}
	g.counts[xdsresource.ClusterTypeURL] = len(targetResources)
	return nil
}

// getEDS 按 getCDS 保存的名称获取 ClusterLoadAssignment（EDS）。
func (g *compatibilityGate) getEDS() error {
	response, err := g.requestResource(xdsresource.EndpointTypeURL, g.endpointNames)
	if err != nil {
		return err
	}
	resources, err := xdsresource.UnmarshalEDS(response.GetResources())
	if err != nil {
		return fmt.Errorf("decode EDS: %w", err)
	}
	targetResources := filterResources(resources, g.endpointNames)
	log.Printf("EDS target resource names=%v", sortedNames(targetResources))
	if err := g.acknowledge(response, g.endpointNames); err != nil {
		return err
	}
	g.counts[xdsresource.EndpointTypeURL] = len(targetResources)
	return nil
}

// buildClusterNames 根据目标服务、端口和 subset 构造 CDS 的精确资源名称。
// 例如 xhs-service 的 80 端口和 v1/v2 subset 会生成三个 Cluster 名称。
func buildClusterNames(service string, port int, subsets string) []string {
	names := []string{fmt.Sprintf("outbound|%d||%s", port, service)}
	for _, subset := range splitCSV(subsets) {
		names = append(names, fmt.Sprintf("outbound|%d|%s|%s", port, subset, service))
	}
	return names
}

// splitCSV 将环境变量中的逗号分隔 subset 转换为去空格、去重后的列表。
func splitCSV(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			values = appendUnique(values, item)
		}
	}
	return values
}

// requestResource 发送一种资源的订阅请求，并接收对应的 DiscoveryResponse。
// NDS 使用完整 NameTable；LDS、CDS、RDS、EDS 使用目标资源名称精确订阅。
func (g *compatibilityGate) requestResource(typeURL string, resourceNames []string) (*discoveryv3.DiscoveryResponse, error) {
	// 保存该类型的精确订阅名称。ADS 是异步流，等待目标响应时可能先收到其他类型的更新，
	// 这些更新也必须用当前订阅名称 ACK，不能被误认为目标响应。
	g.subscriptions[typeURL] = resourceNames
	if err := g.stream.Send(&discoveryv3.DiscoveryRequest{
		Node:          g.node,
		TypeUrl:       typeURL,
		ResourceNames: resourceNames,
	}); err != nil {
		return nil, fmt.Errorf("request %s: %w", shortType(typeURL), err)
	}
	for {
		response, err := g.stream.Recv()
		if err != nil {
			return nil, fmt.Errorf("receive %s: %w", shortType(typeURL), err)
		}
		if response.GetTypeUrl() == typeURL {
			log.Printf("compatible type=%s version=%q resources=%d", shortType(typeURL), response.GetVersionInfo(), len(response.GetResources()))
			return response, nil
		}

		// 处理 ADS 流中先到达的异步更新，保持流状态后继续等待目标类型。
		log.Printf("ignore unsolicited type=%s while waiting for %s", shortType(response.GetTypeUrl()), shortType(typeURL))
		if err := g.validateResponse(response); err != nil {
			return nil, err
		}
		if err := g.acknowledge(response, g.subscriptions[response.GetTypeUrl()]); err != nil {
			return nil, err
		}
	}
}

// acknowledge 告知 istiod 已成功解析当前版本。ACK 必须携带对应的版本、nonce 和订阅名称。
func (g *compatibilityGate) acknowledge(response *discoveryv3.DiscoveryResponse, resourceNames []string) error {
	if err := g.stream.Send(&discoveryv3.DiscoveryRequest{
		VersionInfo:   response.GetVersionInfo(),
		Node:          g.node,
		TypeUrl:       response.GetTypeUrl(),
		ResourceNames: resourceNames,
		ResponseNonce: response.GetNonce(),
	}); err != nil {
		return fmt.Errorf("ACK %s: %w", shortType(response.GetTypeUrl()), err)
	}
	return nil
}

// validateResponse 使用 kitex-contrib/xds 的解码器验证异步响应，避免发送无效 ACK。
func (g *compatibilityGate) validateResponse(response *discoveryv3.DiscoveryResponse) error {
	var err error
	switch response.GetTypeUrl() {
	case xdsresource.NameTableTypeURL:
		_, err = xdsresource.UnmarshalNDS(response.GetResources())
	case xdsresource.ListenerTypeURL:
		_, err = xdsresource.UnmarshalLDS(response.GetResources())
	case xdsresource.RouteTypeURL:
		_, err = xdsresource.UnmarshalRDS(response.GetResources())
	case xdsresource.ClusterTypeURL:
		_, err = xdsresource.UnmarshalCDS(response.GetResources())
	case xdsresource.EndpointTypeURL:
		_, err = xdsresource.UnmarshalEDS(response.GetResources())
	default:
		return fmt.Errorf("unexpected type URL %q", response.GetTypeUrl())
	}
	if err != nil {
		return fmt.Errorf("decode unsolicited %s: %w", shortType(response.GetTypeUrl()), err)
	}
	return nil
}

// sortedNames 提取资源 map 的 key 并排序，保证日志输出稳定、便于对比多次探针结果。
func sortedNames[T any](resources map[string]T) []string {
	names := make([]string, 0, len(resources))
	for name := range resources {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// filterResources 只保留订阅目标对应的资源。即使控制面返回了更多资源，日志和统计也不展示它们。
func filterResources[T any](resources map[string]T, wanted []string) map[string]T {
	filtered := make(map[string]T, len(wanted))
	for _, name := range wanted {
		if resource, ok := resources[name]; ok {
			filtered[name] = resource
		}
	}
	return filtered
}

// filterNames 从资源名称中保留指定的名称，并保持排序后的稳定输出。
func filterNames[T any](resources map[string]T, wanted []string) []string {
	return sortedNames(filterResources(resources, wanted))
}

// appendUnique 去重，避免同一个 RDS/EDS 名称被重复订阅。
func appendUnique(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

// requiredEnv 读取必须的环境变量；Pod 身份字段缺失时无法让 istiod 下发正确配置。
func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("required environment variable %s is empty", name)
	}
	return value
}

// getenv 读取可选环境变量，为空时返回 fallback。
func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// shortType 将完整 TypeURL 截断为日志中的简短资源类型名。
// 例如 type.googleapis.com/envoy.config.listener.v3.Listener -> Listener。
func shortType(typeURL string) string {
	for index := len(typeURL) - 1; index >= 0; index-- {
		if typeURL[index] == '.' {
			return typeURL[index+1:]
		}
	}
	return typeURL
}
