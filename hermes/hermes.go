package hermes

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/lotharbach/tesla-hermesclient/protos"
	log "github.com/sirupsen/logrus"
	"github.com/teslamotors/vehicle-command/pkg/connector"
	"github.com/teslamotors/vehicle-command/pkg/protocol/protobuf/universalmessage"
	"google.golang.org/protobuf/proto"
)

var serverURL = "wss://signaling.vn.teslamotors.com:443/v1/mobile"

type Connection struct {
	vin          string
	inbox        chan []byte
	client       *websocket.Conn
	lock         sync.Mutex
	userToken    string
	vehicleToken string
	errChan      chan error
}

func webSocketConnect(serverURL, userToken string) (*websocket.Conn, error) {
	log.Debug("Opening Websocket")

	headers := http.Header{
		"X-Jwt": {userToken},
	}
	client, _, err := websocket.DefaultDialer.Dial(serverURL, headers)

	if err != nil {
		return nil, err
	}

	return client, err
}

func NewConnection(vin, userToken, vehicleToken string) (*Connection, error) {
	var err error

	client, err := webSocketConnect(serverURL, userToken)
	if err != nil {
		return nil, err
	}

	conn := Connection{
		vin:          vin,
		client:       client,
		inbox:        make(chan []byte, 5),
		userToken:    userToken,
		vehicleToken: vehicleToken,
		errChan:      make(chan error, 1),
	}

	go func() {
		for {
			_, message, err := client.ReadMessage()
			if err != nil {
				conn.errChan <- err
			}

			conn.newMessage(message)
		}
	}()

	select {
	case err := <-conn.errChan:
		return &conn, err
	default:
	}

	return &conn, err
}

func (c *Connection) newMessage(buffer []byte) error {
	var err error
	message := &protos.HermesMessage{}
	if err := proto.Unmarshal(buffer, message); err != nil {
		return err
	}
	log.Debug("Received from server " + string(message.GetCommandMessage().String()))
	payload := message.CommandMessage.GetPayload()

	if !statusCodeOK(message.GetCommandMessage().GetStatusCode()) {
		return fmt.Errorf("Received Status NOT OK: " + string(message.GetCommandMessage().GetPayload()))
	}

	// Put only command response payloads into the inbox
	if message.GetCommandMessage().GetCommandType() == *protos.CommandType_COMMAND_TYPE_SIGNED_COMMAND_RESPONSE.Enum() {

		decoded := &universalmessage.RoutableMessage{}
		if err := proto.Unmarshal(payload, decoded); err != nil {
			return err
		}
		log.Debug("Decoded Payload: " + decoded.String())

		select {
		case c.inbox <- payload:
		default:
			return fmt.Errorf("dropped response due to full inbox")
		}
	}
	// ACK every message
	err = c.sendAck(message)
	if err != nil {
		return err
	}
	return nil
}

func (c *Connection) sendAck(input *protos.HermesMessage) error {
	requestTxid := input.GetCommandMessage().GetTxid()
	topic := input.GetCommandMessage().GetTopic()
	uuid := uuid.New()
	output := &protos.HermesMessage{
		CommandMessage: &protos.CommandMessage{
			Txid:        []byte(uuid.String()),
			Topic:       topic,
			RequestTxid: requestTxid,
			StatusCode:  *protos.StatusCode_STATUS_CODE_CLIENT_ACK.Enum(),
		},
	}

	err := c.sendMessage(output)
	if err != nil {
		return err
	}
	return nil
}

func (c *Connection) sendMessage(output *protos.HermesMessage) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	log.Debug("Sending Message: " + output.String())

	encoded, err := proto.Marshal(output)
	if err != nil {
		return err
	}

	err = c.client.WriteMessage(websocket.BinaryMessage, encoded)
	if err != nil {
		return err
	}
	return nil
}

func (c *Connection) PreferredAuthMethod() connector.AuthMethod {
	return connector.AuthMethodHMAC
}

func (c *Connection) RetryInterval() time.Duration {
	return time.Second
}

func (c *Connection) Receive() <-chan []byte {
	return c.inbox
}

func (c *Connection) Close() {
	c.client.Close()
	if c.inbox != nil {
		close(c.inbox)
		c.inbox = nil
	}
}

func statusCodeOK(code protos.StatusCode) bool {
	switch code {
	case
		*protos.StatusCode_STATUS_CODE_OK.Enum(),
		*protos.StatusCode_STATUS_CODE_CLIENT_ACK.Enum(),
		*protos.StatusCode_STATUS_CODE_SERVER_ACK.Enum(),
		*protos.StatusCode_STATUS_CODE_APPLICATION_OK.Enum(),
		*protos.StatusCode_STATUS_CODE_APPLICATION_ACK.Enum():
		return true
	}
	return false
}

func (c *Connection) VIN() string {
	return c.vin
}

func (c *Connection) Send(ctx context.Context, buffer []byte) error {

	topic := "vehicle_device." + c.vin + ".cmds"
	uuid := uuid.New()
	output := &protos.HermesMessage{
		CommandMessage: &protos.CommandMessage{
			Txid:    []byte(uuid.String()),
			Topic:   []byte(topic),
			Expiry:  &protos.Timestamp{Seconds: 10},
			Payload: buffer,
			Options: &protos.FlatbuffersMessageOptions{
				Token: []byte(c.vehicleToken),
			},
		},
	}

	err := c.sendMessage(output)
	if err != nil {
		return err
	}
	return nil
}
