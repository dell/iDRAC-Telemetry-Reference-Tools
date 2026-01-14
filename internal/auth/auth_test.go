package auth

import (
	"encoding/json"
	"testing"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/mock"
)

func TestReadOneMessage(t *testing.T) {
	expectedMessage := "Check"
	jsonExpectedMessage, _ := json.Marshal(expectedMessage)
	mb := &mock.MockMessageBus{
		Messages: []string{string(jsonExpectedMessage), "\"foo\"", "bar"},
	}
	authClient := new(AuthorizationClient)
	authClient.Bus = mb
	var gotMessage *string
	err := authClient.ReadOneMessage("", &gotMessage)
	if err != nil {
		t.Errorf("ReadOneMessage() error = %v", err)
	}
	if gotMessage == nil || *gotMessage != expectedMessage {
		t.Errorf("ReadOneMessage() = %v, want %v", gotMessage, expectedMessage)
	}

	// Test timeout condition
	emptyMb := &mock.MockMessageBus{
		Messages: []string{},
	}
	authClient.Bus = emptyMb
	var timeoutMessage *string
	err = authClient.ReadOneMessage("", &timeoutMessage)
	if err == nil {
		t.Errorf("ReadOneMessage() expected timeout error, got nil")
	}
}

func TestRequestReply(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		expectedReply := "ok"
		jsonExpectedReply, _ := json.Marshal(expectedReply)
		mb := &mock.MockMessageBus{
			Messages: []string{string(jsonExpectedReply)},
		}
		authClient := new(AuthorizationClient)
		authClient.Bus = mb

		recvQueue := "/temp-queue/test"
		cmd := Command{Command: "testcmd", ReceiveQueue: recvQueue}

		var gotReply string
		err := authClient.requestReply(cmd, &gotReply)
		if err != nil {
			t.Fatalf("requestReply() error = %v", err)
		}
		if gotReply != expectedReply {
			t.Fatalf("requestReply() got reply %q, want %q", gotReply, expectedReply)
		}

		if len(mb.Messages) != 1 {
			t.Fatalf("expected 1 sent message, got %d", len(mb.Messages))
		}
		var gotCmd Command
		if err := json.Unmarshal([]byte(mb.Messages[0]), &gotCmd); err != nil {
			t.Fatalf("failed to unmarshal sent command: %v", err)
		}
		if gotCmd.Command != cmd.Command {
			t.Fatalf("sent command = %q, want %q", gotCmd.Command, cmd.Command)
		}
		if gotCmd.ReceiveQueue != cmd.ReceiveQueue {
			t.Fatalf("sent ReceiveQueue = %q, want %q", gotCmd.ReceiveQueue, cmd.ReceiveQueue)
		}
	})

	t.Run("missing receivequeue", func(t *testing.T) {
		mb := &mock.MockMessageBus{}
		authClient := new(AuthorizationClient)
		authClient.Bus = mb

		err := authClient.requestReply(Command{Command: "testcmd"}, new(string))
		if err == nil {
			t.Fatalf("requestReply() expected error, got nil")
		}
	})
}

func TestGetServiceItems(t *testing.T) {
	authClient := new(AuthorizationClient)
	expectedServiceItems := []ServiceItem{
		{
			Service: Service{
				Ip: "foo",
			},
			ServiceIP: "sip1",
		},
		{
			Service: Service{
				Ip: "bar",
			},
			ServiceIP: "sip2",
		},
	}
	jsonExpSerItems, _ := json.Marshal(expectedServiceItems)
	mb := &mock.MockMessageBus{
		Messages: []string{string(jsonExpSerItems)},
	}
	authClient.Bus = mb
	serviceItems := authClient.GetServiceItems("sip1")
	if len(serviceItems) != 2 {
		t.Errorf("GetServiceItems() = %v, want %v", len(serviceItems), len(expectedServiceItems))
	}
	t.Log(serviceItems)
	t.Log(mb.Messages)
}

func TestAddServiceItem(t *testing.T) {
	authClient := new(AuthorizationClient)
	mb := &mock.MockMessageBus{}
	authClient.Bus = mb
	expectedSI := ServiceItem{
		Service: Service{
			Ip: "foo",
		},
		ServiceIP: "sip1",
	}
	authClient.AddServiceItem(expectedSI)

	var gotCom *Command
	err := authClient.ReadOneMessage("", &gotCom)
	if err != nil {
		t.Errorf("ReadOneMessage() error = %v", err)
	}
	if gotCom == nil || gotCom.ServiceItem.ServiceIP != expectedSI.ServiceIP {
		t.Errorf("ReadOneMessage() = %v, want %v", gotCom, expectedSI)
	}
	if gotCom.Command != ADDSERVICEITEM {
		t.Errorf("ReadOneMessage() = %v, want %v", gotCom, ADDSERVICEITEM)
	}
}

func TestUpdateServiceItem(t *testing.T) {
	authClient := new(AuthorizationClient)
	mb := &mock.MockMessageBus{}
	authClient.Bus = mb
	expectedSI := ServiceItem{
		Service: Service{
			Ip: "foo",
		},
		ServiceIP: "sip1",
	}
	authClient.UpdateServiceItem(expectedSI)

	var gotCom *Command
	err := authClient.ReadOneMessage("", &gotCom)
	if err != nil {
		t.Errorf("ReadOneMessage() error = %v", err)
	}
	if gotCom == nil || gotCom.ServiceItem.ServiceIP != expectedSI.ServiceIP {
		t.Errorf("ReadOneMessage() = %v, want %v", gotCom, expectedSI)
	}
	if gotCom.Command != UPDATESERVICEITEM {
		t.Errorf("ReadOneMessage() = %v, want %v", gotCom, UPDATESERVICEITEM)
	}
}

func TestDeleteServiceItem(t *testing.T) {
	authClient := new(AuthorizationClient)
	mb := &mock.MockMessageBus{}
	authClient.Bus = mb
	expectedSI := ServiceItem{
		Service: Service{
			Ip: "foo",
		},
		ServiceIP: "sip1",
	}
	authClient.DeleteServiceItem(expectedSI)

	var gotCom *Command
	err := authClient.ReadOneMessage("", &gotCom)
	if err != nil {
		t.Errorf("ReadOneMessage() error = %v", err)
	}
	if gotCom == nil || gotCom.ServiceItem.ServiceIP != expectedSI.ServiceIP {
		t.Errorf("ReadOneMessage() = %v, want %v", gotCom, expectedSI)
	}
	if gotCom.Command != DELETESERVICEITEM {
		t.Errorf("ReadOneMessage() = %v, want %v", gotCom, DELETESERVICEITEM)
	}
}
