package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
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

// adsStream 是 Kitex ADS 双向流需要的最小接口。
// 将流抽象为接口后，每个 getXDS 方法都可以专注于一种 Discovery Service。
type adsStream interface {
	Send(*discoveryv3.DiscoveryRequest) error
	Recv() (*discoveryv3.DiscoveryResponse, error)
}

// compatibilityGate 保存一次兼容性检查所需的全部状态。
// routeNames 和 endpointNames 分别保存 LDS -> RDS、CDS -> EDS 的依赖名称；
// counts 保存每类资源收到的数量，避免在 main 中传递多个中间结果。
type compatibilityGate struct {
	stream adsStream
	node   *corev3.Node

	routeNames    []string
	endpointNames []string
	counts        map[string]int
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

	// ADS（Aggregated Discovery Service）使用一条双向流传输 NDS、LDS、RDS、CDS、EDS。
	stream, err := adsClient.StreamAggregatedResources(ctx)
	if err != nil {
		log.Fatalf("open ADS stream to %s: %v", istiodAddress, err)
	}

	// main 只负责组装兼容性检查对象并启动流程；资源获取和状态维护由结构体完成。
	gate := &compatibilityGate{
		stream: stream,
		node:   node,
		counts: make(map[string]int, 5),
	}
	if err := gate.run(); err != nil {
		log.Fatal(err)
	}
}

// run 按 NDS -> LDS -> RDS、CDS -> EDS 的依赖顺序调用各类 Discovery Service 方法。
func (g *compatibilityGate) run() error {
	if err := g.getNDS(); err != nil {
		return err
	}
	if err := g.getLDS(); err != nil {
		return err
	}
	if err := g.getRDS(); err != nil {
		return err
	}
	if err := g.getCDS(); err != nil {
		return err
	}
	if err := g.getEDS(); err != nil {
		return err
	}

	// 五类资源都成功解析并 ACK，说明当前 Kitex xDS 实现通过基础兼容性门禁。
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
	log.Printf("NDS resource names=%v", sortedNames(resources.NameTable))
	return g.saveACK(response, nil)
}

// getLDS 获取 Listener（LDS），并提取 Listener 引用的 RouteConfiguration 名称。
// 提取出的名称保存到结构体，会作为 getRDS 的精确 ResourceNames。
func (g *compatibilityGate) getLDS() error {
	response, err := g.requestResource(xdsresource.ListenerTypeURL, nil)
	if err != nil {
		return err
	}
	resources, err := xdsresource.UnmarshalLDS(response.GetResources())
	if err != nil {
		return fmt.Errorf("decode LDS: %w", err)
	}
	log.Printf("LDS resource names=%v", sortedNames(resources))

	for _, listener := range resources {
		for _, filter := range listener.NetworkFilters {
			if filter.RouteConfigName != "" {
				g.routeNames = appendUnique(g.routeNames, filter.RouteConfigName)
			}
		}
	}
	sort.Strings(g.routeNames)
	return g.saveACK(response, nil)
}

// getRDS 按 getLDS 保存的名称获取 RouteConfiguration（RDS）。
func (g *compatibilityGate) getRDS() error {
	response, err := g.requestResource(xdsresource.RouteTypeURL, g.routeNames)
	if err != nil {
		return err
	}
	resources, err := xdsresource.UnmarshalRDS(response.GetResources())
	if err != nil {
		return fmt.Errorf("decode RDS: %w", err)
	}
	log.Printf("RDS resource names=%v", sortedNames(resources))
	return g.saveACK(response, g.routeNames)
}

// getCDS 获取 Cluster（CDS），并提取 EDS 类型集群引用的 endpoint 名称。
// 提取出的名称保存到结构体，会作为 getEDS 的精确 ResourceNames。
func (g *compatibilityGate) getCDS() error {
	response, err := g.requestResource(xdsresource.ClusterTypeURL, nil)
	if err != nil {
		return err
	}
	resources, err := xdsresource.UnmarshalCDS(response.GetResources())
	if err != nil {
		return fmt.Errorf("decode CDS: %w", err)
	}
	log.Printf("CDS resource names=%v", sortedNames(resources))

	for _, resource := range resources {
		cluster, ok := resource.(*xdsresource.ClusterResource)
		if ok && cluster.DiscoveryType == xdsresource.ClusterDiscoveryTypeEDS && cluster.EndpointName != "" {
			g.endpointNames = appendUnique(g.endpointNames, cluster.EndpointName)
		}
	}
	sort.Strings(g.endpointNames)
	return g.saveACK(response, nil)
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
	log.Printf("EDS resource names=%v", sortedNames(resources))
	return g.saveACK(response, g.endpointNames)
}

// requestResource 发送一种资源的订阅请求，并接收对应的 DiscoveryResponse。
// NDS/LDS/CDS 使用空名称请求初始资源；RDS/EDS 使用父资源提取出的精确名称。
func (g *compatibilityGate) requestResource(typeURL string, resourceNames []string) (*discoveryv3.DiscoveryResponse, error) {
	if err := g.stream.Send(&discoveryv3.DiscoveryRequest{
		Node:          g.node,
		TypeUrl:       typeURL,
		ResourceNames: resourceNames,
	}); err != nil {
		return nil, fmt.Errorf("request %s: %w", shortType(typeURL), err)
	}
	response, err := g.stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("receive %s: %w", shortType(typeURL), err)
	}
	if response.GetTypeUrl() != typeURL {
		return nil, fmt.Errorf("expected %s response, got %s", shortType(typeURL), shortType(response.GetTypeUrl()))
	}
	log.Printf("compatible type=%s version=%q resources=%d", shortType(typeURL), response.GetVersionInfo(), len(response.GetResources()))
	return response, nil
}

// saveACK 告知 istiod 已成功解析当前版本，并保存该类型的资源数量。
// ACK 必须携带对应的版本、nonce 和精确订阅名称。
func (g *compatibilityGate) saveACK(response *discoveryv3.DiscoveryResponse, resourceNames []string) error {
	if err := g.stream.Send(&discoveryv3.DiscoveryRequest{
		VersionInfo:   response.GetVersionInfo(),
		Node:          g.node,
		TypeUrl:       response.GetTypeUrl(),
		ResourceNames: resourceNames,
		ResponseNonce: response.GetNonce(),
	}); err != nil {
		return fmt.Errorf("ACK %s: %w", shortType(response.GetTypeUrl()), err)
	}
	g.counts[response.GetTypeUrl()] = len(response.GetResources())
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
