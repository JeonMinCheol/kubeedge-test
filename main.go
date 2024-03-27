package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"jmc0504/configuration"
	"os"
	"strings"
	"sync"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/golang/glog"
)

const (
	modelName                 = "test"
	deviceStatus              = "say_something"
	DeviceETPrefix            = "$hw/events/device/"
	DeviceETStateUpdateSuffix = "/state/update"
	TwinETUpdateSuffix        = "/twin/update"
	TwinETCloudSyncSuffix     = "/twin/cloud_updated"
	TwinETGetResultSuffix     = "/twin/get/result"
	TwinETGetSuffix           = "/twin/get"
)

var Token_client Token
var ClientOpts *MQTT.ClientOptions
var Client MQTT.Client
var wg sync.WaitGroup
var deviceTwinResult DeviceTwinUpdate
var deviceID string
var configFile configuration.ReadConfigFile

// Token interface to validate the MQTT connection.
type Token interface {
	Wait() bool
	WaitTimeout(time.Duration) bool
	Error() error
}

// DeviceStateUpdate is the structure used in updating the device state
type DeviceStateUpdate struct {
	State string `json:"state,omitempty"`
}

// BaseMessage the base struct of event message
type BaseMessage struct {
	EventID   string `json:"event_id"`
	Timestamp int64  `json:"timestamp"`
}

// TwinValue the struct of twin value
type TwinValue struct {
	Value    *string        `json:"value, omitempty"`
	Metadata *ValueMetadata `json:"metadata,omitempty"`
}

// ValueMetadata the meta of value
type ValueMetadata struct {
	Timestamp int64 `json:"timestamp, omitempty"`
}

// TypeMetadata the meta of value type
type TypeMetadata struct {
	Type string `json:"type,omitempty"`
}

// TwinVersion twin version
type TwinVersion struct {
	CloudVersion int64 `json:"cloud"`
	EdgeVersion  int64 `json:"edge"`
}

// MsgTwin the struct of device twin
type MsgTwin struct {
	Expected        *TwinValue    `json:"expected,omitempty"`
	Actual          *TwinValue    `json:"actual,omitempty"`
	Optional        *bool         `json:"optional,omitempty"`
	Metadata        *TypeMetadata `json:"metadata,omitempty"`
	ExpectedVersion *TwinVersion  `json:"expected_version,omitempty"`
	ActualVersion   *TwinVersion  `json:"actual_version,omitempty"`
}

// DeviceTwinUpdate the struct of device twin update
type DeviceTwinUpdate struct {
	BaseMessage
	Twin map[string]*MsgTwin `json:"twin"`
}

// usage is responsible for setting up the default settings of all defined command-line flags for glog.
func usage() {
	flag.PrintDefaults()
	os.Exit(2)
}

// init for getting command line arguments for glog and initiating the MQTT connection
func init() {
	flag.Usage = usage
	// NOTE: This next line is key you have to call flag.Parse() for the command line
	// options or "flags" that are defined in the glog module to be picked up.
	flag.Parse()
	err := configFile.ReadFromConfigFile()
	if err != nil {
		glog.Error(errors.New("Error while reading from config file " + err.Error()))
		os.Exit(1)
	}
	ClientOpts = HubClientInit(configFile.MQTTURL, "eventbus", "", "")
	Client = MQTT.NewClient(ClientOpts)
	if Token_client = Client.Connect(); Token_client.Wait() && Token_client.Error() != nil {
		glog.Error("client.Connect() Error is ", Token_client.Error())
	}
	err = LoadConfigMap()
	if err != nil {
		glog.Error(errors.New("Error while reading from config map " + err.Error()))
		os.Exit(1)
	}
}

// MQTT client 설정
// MQTT 서버, clientID, username, password를 설정한 후 해당 옵션을 반환
func HubClientInit(server, clientID, username, password string) *MQTT.ClientOptions {
	opts := MQTT.NewClientOptions().AddBroker(server).SetClientID(clientID).SetCleanSession(true)
	if username != "" {
		opts.SetUsername(username)
		if password != "" {
			opts.SetPassword(password)
		}
	}
	tlsConfig := &tls.Config{InsecureSkipVerify: true, ClientAuth: tls.NoClientCert} // 인증 절차 제외
	opts.SetTLSConfig(tlsConfig)
	return opts
}

func LoadConfigMap() error {
	// var ok bool
	readConfigMap := configuration.DeviceProfile{}
	err := readConfigMap.ReadFromConfigMap()
	if err != nil {
		return errors.New("Error while reading from config map " + err.Error())
	}
	for _, device := range readConfigMap.DeviceInstances {
		if strings.ToUpper(device.Model) == modelName && strings.ToUpper(device.Name) == strings.ToUpper(configFile.DeviceName) {
			deviceID = device.ID
		}
	}
	// for _, deviceModel := range readConfigMap.DeviceModels {
	// 	if strings.ToUpper(deviceModel.Name) == modelName {
	// 		for _, property := range deviceModel.Properties {
	// 			if strings.ToUpper(property.Name) == pinNumberConfig {
	// 				if pinNumber, ok = property.DefaultValue.(float64); ok == false {
	// 					return errors.New(" Error in reading pin number from config map")
	// 				}
	// 			}
	// 		}
	// 	}
	// }
	return nil
}

// Device의 상태를 변경하는데 사용.
func changeDeviceState(state string) {
	glog.Info("Changing the state of the device to online")
	var deviceStateUpdateMessage DeviceStateUpdate
	deviceStateUpdateMessage.State = state
	stateUpdateBody, err := json.Marshal(deviceStateUpdateMessage)
	if err != nil {
		glog.Error("Error:   ", err)
	}
	deviceStatusUpdate := DeviceETPrefix + deviceID + DeviceETStateUpdateSuffix
	Token_client = Client.Publish(deviceStatusUpdate, 0, false, stateUpdateBody)
	if Token_client.Wait() && Token_client.Error() != nil {
		glog.Error("client.publish() Error in device state update  is ", Token_client.Error())
	}
}

// MQTT broker를 통해 twin value를 edge에게 전송
func changeTwinValue(updateMessage DeviceTwinUpdate) {
	twinUpdateBody, err := json.Marshal(updateMessage)
	if err != nil {
		glog.Error("Error:   ", err)
	}
	deviceTwinUpdate := DeviceETPrefix + deviceID + TwinETUpdateSuffix        // $hw/events/device/deviceId/twin/update
	Token_client = Client.Publish(deviceTwinUpdate, 0, false, twinUpdateBody) // topic, qos, retained, payload (retain은 메세지 유지 여부, true시 새 클라이언트에게도 전달)
	if Token_client.Wait() && Token_client.Error() != nil {
		glog.Error("client.publish() Error in device twin update is ", Token_client.Error())
	}
}

// 메세지를 전달받으면 수행되는 콜백 함수
// byte[] 메세지를 deviceTwinResult 객체로 변환
// deviceTwinResult는 전역 변수
func OnSubMessageReceived(client MQTT.Client, message MQTT.Message) {
	err := json.Unmarshal(message.Payload(), &deviceTwinResult)
	if err != nil {
		glog.Error("Error in unmarshalling:  ", err)
	}
}

// device twin 업데이트 메세지를 생성할 때 사용
// actualValue를 전달받아 DeviceTwinUpdate 객체를 반환
func createActualUpdateMessage(actualValue string) DeviceTwinUpdate {
	var deviceTwinUpdateMessage DeviceTwinUpdate
	actualMap := map[string]*MsgTwin{deviceStatus: {Actual: &TwinValue{Value: &actualValue}, Metadata: &TypeMetadata{Type: "Updated"}}}
	deviceTwinUpdateMessage.Twin = actualMap
	return deviceTwinUpdateMessage
}

// edge의 device twin 정보를 요청하는 함수
// subscriber에게 정보 전송
func getTwin(updateMessage DeviceTwinUpdate) {
	getTwin := DeviceETPrefix + deviceID + TwinETGetSuffix // $hw/events/device/deviceId/twin/get -> "$hw/events/device/deviceId/twin/get/result"
	twinUpdateBody, err := json.Marshal(updateMessage)
	if err != nil {
		glog.Error("Error:   ", err)
	}
	Token_client = Client.Publish(getTwin, 0, false, twinUpdateBody)
	if Token_client.Wait() && Token_client.Error() != nil {
		glog.Error("client.publish() Error in device twin get  is ", Token_client.Error())
	}
}

// device twin에 대한 정보를 MQTT 브로커를 통해 subscribe
// getTwin 함수를 실행하면 해당 함수로 twin 정보를 받음
func subscribe() {
	for {
		getTwinResult := DeviceETPrefix + deviceID + TwinETGetResultSuffix
		Token_client = Client.Subscribe(getTwinResult, 0, OnSubMessageReceived)
		if Token_client.Wait() && Token_client.Error() != nil {
			glog.Error("subscribe() Error in device twin result get  is ", Token_client.Error())
		}
		time.Sleep(1 * time.Second)
		if deviceTwinResult.Twin != nil {
			wg.Done()
			break
		}
	}
}

// device의 expected state와 actual state를 동기화하는 함수
func equateTwinValue(updateMessage DeviceTwinUpdate) {
	glog.Info("Watching on the device twin values for device: ", configFile.DeviceName)
	wg.Add(1)
	go subscribe()         // 1. 토픽 구독
	getTwin(updateMessage) // 2. 구독한 토픽을 통해 device twin 데이터 요청, 해당 데이터는 deviceTwinResult에 담김
	wg.Wait()

	// 받아온 데이터를 통해 device의 expected state와 actual state를 동기화
	// 아래 코드 잘못된 듯
	// deviceTwinResult.Twin[deviceStatus].Actual == nil이 아니면 실행을 못함;;
	if deviceTwinResult.Twin[deviceStatus].Expected != nil && (deviceTwinResult.Twin[deviceStatus].Actual == nil &&
		deviceTwinResult.Twin[deviceStatus].Expected != nil || (*deviceTwinResult.Twin[deviceStatus].Expected.Value != *deviceTwinResult.Twin[deviceStatus].Actual.Value)) {

		glog.Info("Expected Value : ", *deviceTwinResult.Twin[deviceStatus].Expected.Value)

		glog.Info("Actual Value: ", deviceTwinResult.Twin[deviceStatus].Actual, "Expected Value: ", deviceTwinResult.Twin[deviceStatus].Expected.Value)

		glog.Info("Equating the actual  value to expected value")

		updateMessage = createActualUpdateMessage(*deviceTwinResult.Twin[deviceStatus].Expected.Value)

		changeTwinValue(updateMessage)
	} else {
		glog.Info("Actual values are in sync with Expected value")
	}
}

func main() {
	changeDeviceState("online")
	updateMessage := createActualUpdateMessage("unknown")
	for {
		equateTwinValue(updateMessage)
	}
}
