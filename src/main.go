package main

import (
	"fmt"
)

func main() {

	fmt.Println("Reading from public OPCUA end points")
	opcua_endpoints := getPublicOPCUAEndpoints()
	printEndPoints(opcua_endpoints)
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
		fmt.Println(" ", index, ". ", en)
	}
}
