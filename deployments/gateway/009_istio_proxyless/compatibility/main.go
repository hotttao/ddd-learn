package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/cloudwego/kitex/client"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/kitex-contrib/xds/core/api/kitex_gen/envoy/service/discovery/v3/aggregateddiscoveryservice"
	"github.com/kitex-contrib/xds/core/xdsresource"
	"google.golang.org/protobuf/types/known/structpb"
)

const defaultIstiodAddress = "istiod.istio-system.svc:15010"

// initialTypes 是建立 ADS 流后主动订阅的三类入口资源：
//   - NDS：Istio 的服务名与地址映射；
//   - LDS：当前工作负载应创建的监听器；
//   - CDS：当前工作负载可访问的上游集群。
//
// RDS 和 EDS 不能在这里直接使用空 ResourceNames 请求。它们是按需资源，
// 资源名称需要分别从 LDS 和 CDS 响应中提取后再订阅。
var initialTypes = []string{
	xdsresource.NameTableTypeURL,
	xdsresource.ListenerTypeURL,
	xdsresource.ClusterTypeURL,
}

func main() {
	// 给整个兼容性检查设置上限，避免 istiod 不可达或协议不兼容时 Pod 永久等待。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 这三个值由 Kubernetes Downward API 注入，用于构造 Istio 能识别的 xDS Node。
	podIP := requiredEnv("INSTANCE_IP")
	podName := requiredEnv("POD_NAME")
	namespace := requiredEnv("POD_NAMESPACE")
	istiodAddress := getenv("KITEX_XDS_ISTIO_ADDR", defaultIstiodAddress)

	// Node.Metadata 描述该 xDS 客户端所处的运行环境。Istiod 会结合这些信息，
	// 为当前 namespace 和工作负载计算它应看到的服务发现及流量配置。
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

	// Node ID 使用 Istio sidecar 的标准格式：
	// sidecar~工作负载IP~Pod名.Namespace~Namespace.svc.cluster.local。
	// 虽然本探针不是 Envoy sidecar，但沿用该格式可以让 istiod 按普通工作负载生成配置。
	node := &corev3.Node{
		Id:       fmt.Sprintf("sidecar~%s~%s.%s~%s.svc.cluster.local", podIP, podName, namespace, namespace),
		Cluster:  "kitex-xds-compatibility",
		Metadata: metadata,
	}

	// 使用 kitex-contrib/xds 生成的 Kitex ADS 客户端，而不是 grpc-go 客户端。
	// 因此成功建立流并解析资源，才能证明本实验实际使用的 Kitex xDS 依赖兼容。
	adsClient, err := aggregateddiscoveryservice.NewClient(
		"istiod.istio-system.svc",
		client.WithHostPorts(istiodAddress),
	)
	if err != nil {
		log.Fatalf("create Kitex ADS client: %v", err)
	}

	// ADS（Aggregated Discovery Service）在同一条双向流中传输多种 xDS 资源。
	stream, err := adsClient.StreamAggregatedResources(ctx)
	if err != nil {
		log.Fatalf("open ADS stream to %s: %v", istiodAddress, err)
	}

	// 保存每种资源的订阅名称。后续发送 ACK 时必须继续携带这些名称，
	// 否则会把一个按需订阅意外改回空订阅。
	subscriptions := make(map[string][]string)
	// 首先请求 NDS、LDS、CDS。首次请求没有 VersionInfo 和 ResponseNonce，表示初始订阅。
	for _, typeURL := range initialTypes {
		if err := stream.Send(&discoveryv3.DiscoveryRequest{Node: node, TypeUrl: typeURL}); err != nil {
			log.Fatalf("request %s: %v", shortType(typeURL), err)
		}
	}

	// 最终必须成功接收并解析 NDS、LDS、RDS、CDS、EDS 五类资源。
	received := make(map[string]int)
	for len(received) < 5 {
		response, err := stream.Recv()
		if err != nil {
			log.Fatalf("receive ADS response after %v: %v", received, err)
		}

		name := shortType(response.GetTypeUrl())
		// decode 不只验证 Protobuf 能否被当前 xds 库解析，还会返回依赖资源：
		// LDS -> RDS 的 RouteConfiguration 名称，CDS -> EDS 的 endpoint 名称。
		dependencies, err := decode(response)
		if err != nil {
			log.Fatalf("decode %s version=%q resources=%d: %v", name, response.GetVersionInfo(), len(response.GetResources()), err)
		}
		received[response.GetTypeUrl()] = len(response.GetResources())
		log.Printf("compatible type=%s version=%q resources=%d", name, response.GetVersionInfo(), len(response.GetResources()))
		// 根据父资源中出现的确切名称发起 RDS/EDS 按需订阅。
		for typeURL, resourceNames := range dependencies {
			if _, requested := subscriptions[typeURL]; requested || len(resourceNames) == 0 {
				continue
			}
			subscriptions[typeURL] = resourceNames
			log.Printf("requesting on-demand type=%s resources=%d", shortType(typeURL), len(resourceNames))
			if err := stream.Send(&discoveryv3.DiscoveryRequest{
				Node:          node,
				TypeUrl:       typeURL,
				ResourceNames: resourceNames,
			}); err != nil {
				log.Fatalf("request on-demand %s: %v", shortType(typeURL), err)
			}
		}

		// 能成功解析就向 istiod 返回 ACK：
		// VersionInfo 表示已接受的版本，ResponseNonce 对应本次响应。
		// 如果解析失败，程序会直接退出，相当于兼容性门禁失败，不发送 ACK。
		if err := stream.Send(&discoveryv3.DiscoveryRequest{
			VersionInfo:   response.GetVersionInfo(),
			Node:          node,
			TypeUrl:       response.GetTypeUrl(),
			ResourceNames: subscriptions[response.GetTypeUrl()],
			ResponseNonce: response.GetNonce(),
		}); err != nil {
			log.Fatalf("ACK %s: %v", name, err)
		}
	}

	names := make([]string, 0, len(received))
	for typeURL, count := range received {
		names = append(names, fmt.Sprintf("%s=%d", shortType(typeURL), count))
	}
	sort.Strings(names)
	log.Printf("compatibility gate passed: %v", names)
}

func decode(response *discoveryv3.DiscoveryResponse) (map[string][]string, error) {
	// 必须调用 kitex-contrib/xds 自己的 Unmarshal 方法，而不只是解析通用 Any，
	// 这样才能检查该版本库是否理解 istiod 1.31 实际下发的资源结构。
	switch response.GetTypeUrl() {
	case xdsresource.NameTableTypeURL:
		// NDS：验证 Istio NameTable，可供 Proxyless 客户端进行服务名解析。
		_, err := xdsresource.UnmarshalNDS(response.GetResources())
		return nil, err
	case xdsresource.ListenerTypeURL:
		// LDS：监听器中的 RouteConfigName 指向需要继续订阅的 RDS 资源。
		resources, err := xdsresource.UnmarshalLDS(response.GetResources())
		if err != nil {
			return nil, err
		}
		var names []string
		for _, listener := range resources {
			for _, filter := range listener.NetworkFilters {
				if filter.RouteConfigName != "" {
					names = appendUnique(names, filter.RouteConfigName)
				}
			}
		}
		sort.Strings(names)
		return map[string][]string{xdsresource.RouteTypeURL: names}, nil
	case xdsresource.RouteTypeURL:
		// RDS：验证路由配置能被当前 Kitex xDS 实现解析。
		_, err := xdsresource.UnmarshalRDS(response.GetResources())
		return nil, err
	case xdsresource.ClusterTypeURL:
		// CDS：仅 EDS 类型集群需要继续订阅对应的 endpoint 资源。
		resources, err := xdsresource.UnmarshalCDS(response.GetResources())
		if err != nil {
			return nil, err
		}
		var names []string
		for _, resource := range resources {
			cluster, ok := resource.(*xdsresource.ClusterResource)
			if ok && cluster.DiscoveryType == xdsresource.ClusterDiscoveryTypeEDS && cluster.EndpointName != "" {
				names = appendUnique(names, cluster.EndpointName)
			}
		}
		sort.Strings(names)
		return map[string][]string{xdsresource.EndpointTypeURL: names}, nil
	case xdsresource.EndpointTypeURL:
		// EDS：验证上游实例地址和负载均衡端点能被解析。
		_, err := xdsresource.UnmarshalEDS(response.GetResources())
		return nil, err
	default:
		return nil, fmt.Errorf("unexpected type URL %q", response.GetTypeUrl())
	}
}

func appendUnique(values []string, candidate string) []string {
	// 同一个 RDS/EDS 名称可能被多个 listener/cluster 引用，只订阅一次。
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func requiredEnv(name string) string {
	// Pod 身份字段缺失会导致 istiod 生成错误的视图，因此不提供默认值。
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("required environment variable %s is empty", name)
	}
	return value
}

func getenv(name, fallback string) string {
	// istiod 地址允许覆盖，便于在不同集群或端口配置下复用探针。
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func shortType(typeURL string) string {
	// 将完整 type URL 缩短成日志中更容易阅读的资源类型名。
	for index := len(typeURL) - 1; index >= 0; index-- {
		if typeURL[index] == '.' {
			return typeURL[index+1:]
		}
	}
	return typeURL
}
