package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/debug"
	"github.com/gopcua/opcua/monitor"
	"github.com/gopcua/opcua/ua"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/sdk/metric/aggregator/histogram"
	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	"go.opentelemetry.io/otel/sdk/metric/export/aggregation"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	selector "go.opentelemetry.io/otel/sdk/metric/selector/simple"
)

var globalMeter metric.Meter

func getPublicOPCUAEndpoints() []string {
	epList := []string{
		"opc.tcp://milo.digitalpetri.com:62541/milo",
		"opc.tcp://opcuademo.sterfive.com:26543",
		"opc.tcp://uademo.prosysopc.com:53530/OPCUA/SimulationServer",
		"opc.tcp://opc.mtconnect.org:4840",
		"opc.tcp://opcuaserver.com:48010",
		"opc.tcp://opcuaserver.com:4840",
	}

	return epList
}

func main() {
	var (
		endpoint = flag.String("endpoint", "opc.tcp://127.0.1.1:50000", "OPC UA Endpoint URL")
		policy   = flag.String("policy", "None", "Security policy: None, Basic128Rsa15, Basic256, Basic256Sha256. Default: auto")
		mode     = flag.String("mode", "None", "Security mode: None, Sign, SignAndEncrypt. Default: auto")
		// certFile = flag.String("cert", "", "Path to cert.pem. Required for security mode/policy != None")
		// keyFile  = flag.String("key", "", "Path to private key.pem. Required for security mode/policy != None")
		nodeIDs = flag.String("nodes", "SpikeData,StepUp,DipData,FastNumberOfUpdates,SlowNumberOfUpdates", "node ids to subscribe to, seperated by commas")
		nodePre = flag.String("prefix", "ns=2;", "prefix to add to Node IDs.")
		// interval = flag.String("interval", opcua.DefaultSubscriptionInterval.String(), "subscription interval")
	)

	flag.BoolVar(&debug.Enable, "debug", false, "enable debug logging")
	flag.Parse()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-signalCh
		println()
		cancel()
	}()

	fmt.Println("Configuring OpenTelemetry")
	configureOpentelemetry()
	globalMeter = global.Meter("mis/opcua")

	opcuaEndpoints, err := opcua.GetEndpoints(ctx, *endpoint)
	if err != nil {
		log.Fatal(err)
	}

	ep := opcua.SelectEndpoint(opcuaEndpoints, *policy, ua.MessageSecurityModeFromString(*mode))
	if ep == nil {
		log.Fatal("Failed to find suitable endpoint")
	}

	var nodeList []string
	for _, nodeID := range strings.Split(*nodeIDs, ",") {
		nodeList = append(nodeList, *nodePre+nodeID)
	}

	opcuaOpts := []opcua.Option{
		opcua.SecurityPolicy(*policy),
		opcua.SecurityModeString(*mode),
		// opcua.CertificateFile(*certFile),
		// opcua.PrivateKeyFile(*keyFile),
		opcua.AuthAnonymous(),
		opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeAnonymous),
	}

	// browseEndPoint(endpoint)
	monitorDevices(ctx, *ep, opcuaOpts, nodeList)
}

/* func browseEndPoint(endpoint string) {

	nodeID := "ns=0;i=85"

	ctx := context.Background()

	// Connect
	c := opcua.NewClient(endpoint)
	if err := c.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	nid, err := ua.ParseNodeID(nodeID)
	if err != nil {
		log.Fatalf("invalid node id: %s", nodeID)
	}
	fmt.Printf("nid: %s\n", nid)

	n := c.Node(nid)

	attrs, err := n.Attributes(ua.AttributeIDBrowseName, ua.AttributeIDDataType)
	if err != nil {
		log.Fatalf("invalid attribute: %s", err.Error())
	}
	fmt.Printf("BrowseName: %s; DataType: %s\n", attrs[0].Value, getDataType(attrs[1]))
	fmt.Printf("BrowseName: %s; DataType: %s\n", attrs[1].Value, getDataType(attrs[1]))

	// Get children
	refs, err := n.ReferencedNodes(id.HasComponent, ua.BrowseDirectionForward, ua.NodeClassAll, true)
	if err != nil {
		log.Fatalf("References: %s", err)
	}

	fmt.Printf("Children: %d\n", len(refs))
	for _, rn := range refs {
		fmt.Printf("   %s\n", rn.ID.String())
	}
}

func getDataType(value *ua.DataValue) string {
	if value.Status != ua.StatusOK {
		return value.Status.Error()
	}

	switch value.Value.NodeID().IntID() {
	case id.DateTime:
		return "time.Time"

	case id.Boolean:
		return "bool"

	case id.Int32:
		return "int32"
	}

	return value.Value.NodeID().String()
} */

func monitorDevices(ctx context.Context, endpoint ua.EndpointDescription, opcuaOpts []opcua.Option, nodeIdList []string) {
	// nodeID := "ns=2" + ";s=Dynamic" + "/RandomFloat"

	c := opcua.NewClient(endpoint.EndpointURL, opcuaOpts...)

	if err := c.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	m, err := monitor.NewNodeMonitor(c)
	if err != nil {
		log.Fatal(err)
	}

	m.SetErrorHandler(func(_ *opcua.Client, sub *monitor.Subscription, err error) {
		log.Printf("error: sub=%d err=%s", sub.SubscriptionID(), err.Error())
	})
	wg := &sync.WaitGroup{}

	wg.Add(1)
	go startChanSub(ctx, m, time.Second, 0, wg, nodeIdList...)
	<-ctx.Done()
	wg.Wait()
}

func startChanSub(ctx context.Context, m *monitor.NodeMonitor, interval, lag time.Duration, wg *sync.WaitGroup, nodes ...string) {
	// Activity/error/success counters
	activity_counter, _ := globalMeter.SyncInt64().Counter("mis_opcua_activity_counter")
	error_counter, _ := globalMeter.SyncInt64().Counter("mis_opcua_error_counter")
	success_counter, _ := globalMeter.SyncInt64().Counter("mis_opcua_success_counter")
	last_activity_gauge, _ := globalMeter.AsyncInt64().Gauge("mis_opcua_last_activity_timestamp")

	stringGauge, _ := globalMeter.AsyncInt64().Gauge("mis_opcua_value_string")
	booleanGauge, _ := globalMeter.AsyncInt64().Gauge("mis_opcua_value_boolean")
	integerGauge, _ := globalMeter.AsyncInt64().Gauge("mis_opcua_value_integer")
	floatGauge, _ := globalMeter.AsyncFloat64().Gauge("mis_opcua_value_float")

	ch := make(chan *monitor.DataChangeMessage, 128)
	sub, err := m.ChanSubscribe(ctx, &opcua.SubscriptionParameters{Interval: interval}, ch, nodes...)

	if err != nil {
		log.Fatal(err)
	}

	defer cleanup(ctx, sub, wg)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			activity_counter.Add(ctx, 1)

			if msg.Error != nil {
				log.Printf("[callback] error=%s", msg.Error)
				error_counter.Add(ctx, 1)
			} else {
				stringIdAttribute := attribute.String("nodeid_stringid", msg.NodeID.StringID())
				log.Printf("[callback] node=%s value=%v", msg.NodeID, msg.Value.Value())
				switch msg.Value.Type() {
				case ua.TypeIDBoolean:
					value := int64(0)
					if msg.Value.Bool() {
						value = 1
					}
					booleanGauge.Observe(ctx, value, stringIdAttribute, attribute.Bool("boolean_value", msg.Value.Bool()))
				case ua.TypeIDString:
					stringGauge.Observe(ctx, 1, stringIdAttribute, attribute.String("string_value", msg.Value.String()))
				case ua.TypeIDDouble, ua.TypeIDFloat:
					value := float64(msg.Value.Float())
					floatGauge.Observe(ctx, value, stringIdAttribute)
				case ua.TypeIDSByte, ua.TypeIDInt16, ua.TypeIDInt32, ua.TypeIDInt64:
					value := int64(msg.Value.Int())
					integerGauge.Observe(ctx, value, stringIdAttribute)
				case ua.TypeIDByte, ua.TypeIDUint16, ua.TypeIDUint32, ua.TypeIDUint64:
					// There is no uint gauge?
					value := int64(msg.Value.Int())
					integerGauge.Observe(ctx, value, stringIdAttribute)
				default:
					log.Println()
				}
				last_activity_gauge.Observe(ctx, time.Now().Unix())
				success_counter.Add(ctx, 1)
			}
			time.Sleep(lag)
		}
	}
}

func cleanup(ctx context.Context, sub *monitor.Subscription, wg *sync.WaitGroup) {
	log.Printf("stats: sub=%d delivered=%d dropped=%d", sub.SubscriptionID(), sub.Delivered(), sub.Dropped())
	sub.Unsubscribe(ctx)
	wg.Done()
}

func configureOpentelemetry() {
	exporter := configureMetrics()

	http.HandleFunc("/metrics", exporter.ServeHTTP)
	fmt.Println("listenening on http://localhost:8088/metrics")

	go func() {
		_ = http.ListenAndServe(":8088", nil)
	}()
}

func configureMetrics() *prometheus.Exporter {
	config := prometheus.Config{}

	ctrl := controller.New(
		processor.NewFactory(
			selector.NewWithHistogramDistribution(
				histogram.WithExplicitBoundaries(config.DefaultHistogramBoundaries),
			),
			aggregation.CumulativeTemporalitySelector(),
			processor.WithMemory(true),
		),
	)

	exporter, err := prometheus.New(config, ctrl)
	if err != nil {
		panic(err)
	}

	global.SetMeterProvider(exporter.MeterProvider())

	return exporter
}
