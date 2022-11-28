package main

import "github.com/aws/aws-sdk-go-v2/service/sqs/types"

type queue interface {
	receive() ([]queueMessage, error)
	deleteMessage(m queueMessage) error
}

type queueMessage interface {
	id() string
	body() string
}

type sqsMessage struct {
	message types.Message
}

func (m *sqsMessage) id() string {
	return *m.message.MessageId
}

func (m *sqsMessage) body() string {
	return *m.message.Body
}
