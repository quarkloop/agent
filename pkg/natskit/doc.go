// Package natskit defines Quark's application-side NATS transport contract.
//
// Subjects identify operations. Request payloads carry correlation and domain
// input only; they do not repeat the route selected by the publisher.
package natskit
