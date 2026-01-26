package auth

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/mock"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/service"
)

func TestReadOneMessage(t *testing.T) {
	expectedMessage := "Check"
	jsonExpectedMessage, _ := json.Marshal(expectedMessage)
	mb := mock.NewMockMessageBus()
	mb.AddQueueMessage("test-queue", string(jsonExpectedMessage))
	authClient := &AuthorizationClient{
		BaseClient: service.NewBaseClient(mb, CommandQueue, ReplyPrefix, "test", 5*time.Second),
	}
	var gotMessage *string
	err := authClient.ReadOneMessage("test-queue", &gotMessage)
	if err != nil {
		t.Errorf("ReadOneMessage() error = %v", err)
	}
	if gotMessage == nil || *gotMessage != expectedMessage {
		t.Errorf("ReadOneMessage() = %v, want %v", gotMessage, expectedMessage)
	}

	// Test timeout condition
	emptyMb := mock.NewMockMessageBus()
	authClient = &AuthorizationClient{
		BaseClient: service.NewBaseClient(emptyMb, CommandQueue, ReplyPrefix, "test", 100*time.Millisecond),
	}
	var timeoutMessage *string
	err = authClient.ReadOneMessage("test-queue", &timeoutMessage)
	if err == nil {
		t.Errorf("ReadOneMessage() expected timeout error, got nil")
	}
}

func TestCall(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		expectedReply := "ok"

		mb := mock.NewMockMessageBus()
		mb.SetReplyPayload(expectedReply)

		authClient := NewAuthorizationClient(mb, "test-client")

		var gotReply string
		err := authClient.Call("testcmd", nil, &gotReply)
		if err != nil {
			t.Fatalf("Call() error = %v", err)
		}
		if gotReply != expectedReply {
			t.Fatalf("Call() got reply %q, want %q", gotReply, expectedReply)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		mb := mock.NewMockMessageBus()
		// No reply payload set - should timeout

		authClient := &AuthorizationClient{
			BaseClient: service.NewBaseClient(mb, CommandQueue, ReplyPrefix, "test", 100*time.Millisecond),
		}

		var gotReply string
		err := authClient.Call("testcmd", nil, &gotReply)
		if err == nil {
			t.Fatalf("Call() expected timeout error, got nil")
		}
	})
}

func TestGetServiceItems(t *testing.T) {
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

	mb := mock.NewMockMessageBus()
	mb.SetReplyPayload(expectedServiceItems)

	authClient := NewAuthorizationClient(mb, "test-client")
	serviceItems := authClient.GetServiceItems("sip1")
	if len(serviceItems) != 2 {
		t.Errorf("GetServiceItems() = %v, want %v", len(serviceItems), len(expectedServiceItems))
	}
}

func TestAddServiceItem(t *testing.T) {
	mb := mock.NewMockMessageBus()
	authClient := NewAuthorizationClient(mb, "test-client")
	expectedSI := ServiceItem{
		Service: Service{
			Ip: "foo",
		},
		ServiceIP: "sip1",
	}
	err := authClient.AddServiceItem(expectedSI)
	if err != nil {
		t.Errorf("AddServiceItem() error = %v", err)
	}

	// Verify message was sent to command queue
	messages := mb.GetMessages(CommandQueue)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var gotCom Command
	if err := json.Unmarshal([]byte(messages[0]), &gotCom); err != nil {
		t.Fatalf("failed to unmarshal command: %v", err)
	}
	if gotCom.ServiceItem.ServiceIP != expectedSI.ServiceIP {
		t.Errorf("ServiceIP = %v, want %v", gotCom.ServiceItem.ServiceIP, expectedSI.ServiceIP)
	}
	if gotCom.Command != ADDSERVICEITEM {
		t.Errorf("Command = %v, want %v", gotCom.Command, ADDSERVICEITEM)
	}
}

func TestUpdateServiceItem(t *testing.T) {
	mb := mock.NewMockMessageBus()
	authClient := NewAuthorizationClient(mb, "test-client")
	expectedSI := ServiceItem{
		Service: Service{
			Ip: "foo",
		},
		ServiceIP: "sip1",
	}
	err := authClient.UpdateServiceItem(expectedSI)
	if err != nil {
		t.Errorf("UpdateServiceItem() error = %v", err)
	}

	// Verify message was sent to command queue
	messages := mb.GetMessages(CommandQueue)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var gotCom Command
	if err := json.Unmarshal([]byte(messages[0]), &gotCom); err != nil {
		t.Fatalf("failed to unmarshal command: %v", err)
	}
	if gotCom.ServiceItem.ServiceIP != expectedSI.ServiceIP {
		t.Errorf("ServiceIP = %v, want %v", gotCom.ServiceItem.ServiceIP, expectedSI.ServiceIP)
	}
	if gotCom.Command != UPDATESERVICEITEM {
		t.Errorf("Command = %v, want %v", gotCom.Command, UPDATESERVICEITEM)
	}
}

func TestDeleteServiceItem(t *testing.T) {
	mb := mock.NewMockMessageBus()
	authClient := NewAuthorizationClient(mb, "test-client")
	expectedSI := ServiceItem{
		Service: Service{
			Ip: "foo",
		},
		ServiceIP: "sip1",
	}
	err := authClient.DeleteServiceItem(expectedSI)
	if err != nil {
		t.Errorf("DeleteServiceItem() error = %v", err)
	}

	// Verify message was sent to command queue
	messages := mb.GetMessages(CommandQueue)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var gotCom Command
	if err := json.Unmarshal([]byte(messages[0]), &gotCom); err != nil {
		t.Fatalf("failed to unmarshal command: %v", err)
	}
	if gotCom.ServiceItem.ServiceIP != expectedSI.ServiceIP {
		t.Errorf("ServiceIP = %v, want %v", gotCom.ServiceItem.ServiceIP, expectedSI.ServiceIP)
	}
	if gotCom.Command != DELETESERVICEITEM {
		t.Errorf("Command = %v, want %v", gotCom.Command, DELETESERVICEITEM)
	}
}
