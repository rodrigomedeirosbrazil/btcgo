package p2p

import (
	"context"
	"encoding/json"

	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

// A structure that represents a PubSub Chat Room
type ChatRoom struct {
	// Represents the P2P Host for the ChatRoom
	Host *P2P

	// Represents the channel of incoming messages
	Inbound chan chatmessage
	// Represents the channel of outgoing messages
	Outbound chan string
	// Represents the channel of chat log messages
	Logs chan chatlog

	// Represents the host ID of the peer
	selfid peer.ID

	// Represents the chat room lifecycle context
	pubsubCtx context.Context
	// Represents the chat room lifecycle cancellation function
	pubsubCancel context.CancelFunc
	// Represents the PubSub Topic of the ChatRoom
	pubsubTopic *pubsub.Topic
	// Represents the PubSub Subscription for the topic
	pubsubSub *pubsub.Subscription
}

// A structure that represents a chat message
type chatmessage struct {
	Message  string `json:"message"`
	SenderID string `json:"senderid"`
}

// A structure that represents a chat log
type chatlog struct {
	logprefix string
	logmsg    string
}

// A constructor function that generates and returns a new
// ChatRoom for a given P2PHost, username and roomname
func JoinChatRoom(p2phost *P2P) (*ChatRoom, error) {

	// Create a PubSub topic with the room name
	topic, err := p2phost.PubSub.Join("room-btcfinder")
	// Check the error
	if err != nil {
		return nil, err
	}

	// Subscribe to the PubSub topic
	sub, err := topic.Subscribe()
	// Check the error
	if err != nil {
		return nil, err
	}

	// Create cancellable context
	pubsubctx, cancel := context.WithCancel(context.Background())

	// Create a ChatRoom object
	chatroom := &ChatRoom{
		Host: p2phost,

		Inbound:  make(chan chatmessage),
		Outbound: make(chan string),
		Logs:     make(chan chatlog),

		pubsubCtx:    pubsubctx,
		pubsubCancel: cancel,
		pubsubTopic:  topic,
		pubsubSub:    sub,

		selfid: p2phost.Host.ID(),
	}

	// Start the subscribe loop
	go chatroom.SubLoop()
	// Start the publish loop
	go chatroom.PubLoop()

	// Return the chatroom
	return chatroom, nil
}

// A method of ChatRoom that publishes a chatmessage
// to the PubSub topic until the pubsub context closes
func (chatroom *ChatRoom) PubLoop() {
	for {
		select {
		case <-chatroom.pubsubCtx.Done():
			return

		case message := <-chatroom.Outbound:
			// Create a ChatMessage
			m := chatmessage{
				Message:  message,
				SenderID: chatroom.selfid.Pretty(),
			}

			// Marshal the ChatMessage into a JSON
			messagebytes, err := json.Marshal(m)
			if err != nil {
				chatroom.Logs <- chatlog{logprefix: "puberr", logmsg: "could not marshal JSON"}
				continue
			}

			// Publish the message to the topic
			err = chatroom.pubsubTopic.Publish(chatroom.pubsubCtx, messagebytes)
			if err != nil {
				chatroom.Logs <- chatlog{logprefix: "puberr", logmsg: "could not publish to topic"}
				continue
			}
		}
	}
}

// A method of ChatRoom that continously reads from the subscription
// until either the subscription or pubsub context closes.
// The recieved message is parsed sent into the inbound channel
func (chatroom *ChatRoom) SubLoop() {
	// Start loop
	for {
		select {
		case <-chatroom.pubsubCtx.Done():
			return

		default:
			// Read a message from the subscription
			message, err := chatroom.pubsubSub.Next(chatroom.pubsubCtx)
			// Check error
			if err != nil {
				// Close the messages queue (subscription has closed)
				close(chatroom.Inbound)
				chatroom.Logs <- chatlog{logprefix: "suberr", logmsg: "subscription has closed"}
				return
			}

			// Check if message is from self
			if message.ReceivedFrom == chatroom.selfid {
				continue
			}

			// Declare a ChatMessage
			cm := &chatmessage{}
			// Unmarshal the message data into a ChatMessage
			err = json.Unmarshal(message.Data, cm)
			if err != nil {
				chatroom.Logs <- chatlog{logprefix: "suberr", logmsg: "could not unmarshal JSON"}
				continue
			}

			// Send the ChatMessage into the message queue
			chatroom.Inbound <- *cm
		}
	}
}

// A method of ChatRoom that returns a list
// of all peer IDs connected to it
func (chatroom *ChatRoom) PeerList() []peer.ID {
	// Return the slice of peer IDs connected to chat room topic
	return chatroom.pubsubTopic.ListPeers()
}

// A method of ChatRoom that updates the chat
// room by subscribing to the new topic
func (chatroom *ChatRoom) Exit() {
	defer chatroom.pubsubCancel()

	// Cancel the existing subscription
	chatroom.pubsubSub.Cancel()
	// Close the topic handler
	chatroom.pubsubTopic.Close()
}
