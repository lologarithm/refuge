package main

import (
	"context"
	"log"
	"time"

	mailgun "github.com/mailgun/mailgun-go/v3"
)

func sendMail(mc MailgunConfig, subj, msg string) {
	// Create an instance of the Mailgun Client
	mg := mailgun.NewMailgun(mc.Domain, mc.APIKey)
	// The message object allows you to add attachments and Bcc recipients
	message := mg.NewMessage(mc.Sender, subj, msg, mc.Recipients...)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	// Send the message	with a 10 second timeout
	resp, id, err := mg.Send(ctx, message)
	if err != nil {
		log.Printf("[Error] Failed to send alert: %s", err)
	}

	if id == "" {
		log.Printf("[Error] Failed to send alert, invalid ID: %s", resp)
	}
}
