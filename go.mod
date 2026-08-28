module com.tyza66.SimpleMqttBrokerApi

go 1.23.5

require github.com/FasterEdge/MqttBrokerCore v0.0.0

replace github.com/FasterEdge/MqttBrokerCore => ../MqttBrokerCore

require (
	github.com/bwmarrin/snowflake v0.3.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/net v0.40.0 // indirect
)
