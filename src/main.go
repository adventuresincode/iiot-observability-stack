package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/monitor"
	"github.com/gopcua/opcua/ua"
)

func main() {

	fmt.Println("Reading from public OPCUA end points")
	opcua_endpoints := getPublicOPCUAEndpoints()
	printEndPoints(opcua_endpoints)
	browseEndPoint(opcua_endpoints[0])
	monitorDevices(opcua_endpoints[0])
}

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

func printEndPoints(opcua_endpoints []string) {

	for index, en := range opcua_endpoints {
		fmt.Println(" ", index+1, ". ", en)
	}
}

func browseEndPoint(endpoint string) {

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

	n := c.Node(nid)

	// attrs, err := n.Attributes(ua.AttributeIDBrowseName, ua.AttributeIDDataType)
	// if err != nil {
	// 	log.Fatalf("invalid attribute: %s", err.Error())
	// }
	//fmt.Printf("BrowseName: %s; DataType: %s\n", attrs[0].Value, getDataType(attrs[1]))
	//fmt.Printf("BrowseName: %s; DataType: %s\n", attrs[1].Value, getDataType(attrs[1]))

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
}

func monitorDevices(endpoint string) {
	nodeID := "ns=2;s=Dynamic/RandomFloat"

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-signalCh
		println()
		cancel()
	}()

	c := opcua.NewClient(endpoint, opcua.SecurityMode(ua.MessageSecurityModeNone))

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
	go startCallbackSub(ctx, m, time.Second, 0, wg, nodeID)
	<-ctx.Done()
	wg.Wait()
}

func cleanup(ctx context.Context, sub *monitor.Subscription, wg *sync.WaitGroup) {
	log.Printf("stats: sub=%d delivered=%d dropped=%d", sub.SubscriptionID(), sub.Delivered(), sub.Dropped())
	sub.Unsubscribe(ctx)
	wg.Done()
}

func startCallbackSub(ctx context.Context, m *monitor.NodeMonitor, interval, lag time.Duration, wg *sync.WaitGroup, nodes ...string) {
	sub, err := m.Subscribe(
		ctx,
		&opcua.SubscriptionParameters{
			Interval: interval,
		},
		func(s *monitor.Subscription, msg *monitor.DataChangeMessage) {
			if msg.Error != nil {
				log.Printf("[callback] error=%s", msg.Error)
			} else {
				log.Printf("[callback] node=%s value=%v", msg.NodeID, msg.Value.Value())
			}
			time.Sleep(lag)
		},
		nodes...)

	if err != nil {
		log.Fatal(err)
	}

	defer cleanup(ctx, sub, wg)

	<-ctx.Done()
}
