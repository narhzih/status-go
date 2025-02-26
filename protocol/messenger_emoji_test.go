package protocol

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/status-im/status-go/eth-node/crypto"
	"github.com/status-im/status-go/eth-node/types"
	"github.com/status-im/status-go/multiaccounts"
	"github.com/status-im/status-go/protocol/common"
	"github.com/status-im/status-go/protocol/protobuf"
)

func TestMessengerEmojiSuite(t *testing.T) {
	suite.Run(t, new(MessengerEmojiSuite))
}

type MessengerEmojiSuite struct {
	MessengerBaseTestSuite
}

func (s *MessengerEmojiSuite) TestSendEmoji() {
	alice := s.m
	alice.account = &multiaccounts.Account{KeyUID: "0xdeadbeef"}
	key, err := crypto.GenerateKey()
	s.Require().NoError(err)

	bob, err := newMessengerWithKey(s.shh, key, s.logger, nil)
	s.Require().NoError(err)
	defer bob.Shutdown() // nolint: errcheck

	chatID := statusChatID

	chat := CreatePublicChat(chatID, alice.transport)

	err = alice.SaveChat(chat)
	s.Require().NoError(err)

	_, err = alice.Join(chat)
	s.Require().NoError(err)

	err = bob.SaveChat(chat)
	s.Require().NoError(err)

	_, err = bob.Join(chat)
	s.Require().NoError(err)

	// Send chat message from alice to bob

	message := buildTestMessage(*chat)
	_, err = alice.SendChatMessage(context.Background(), message)
	s.NoError(err)

	// Wait for message to arrive to bob
	response, err := WaitOnMessengerResponse(
		bob,
		func(r *MessengerResponse) bool { return len(r.Messages()) > 0 },
		"no messages",
	)
	s.Require().NoError(err)

	s.Require().Len(response.Messages(), 1)

	messageID := response.Messages()[0].ID

	// Respond with an emoji, donald trump style

	response, err = bob.SendEmojiReaction(context.Background(), chat.ID, messageID, protobuf.EmojiReaction_SAD)
	s.Require().NoError(err)
	s.Require().Len(response.EmojiReactions(), 1)

	emojiID := response.EmojiReactions()[0].ID()

	// Wait for the emoji to arrive to alice
	response, err = WaitOnMessengerResponse(
		alice,
		func(r *MessengerResponse) bool { return len(r.EmojiReactions()) == 1 },
		"no emoji",
	)
	s.Require().NoError(err)

	s.Require().Len(response.EmojiReactions(), 1)
	s.Require().Equal(response.EmojiReactions()[0].ID(), emojiID)
	s.Require().Equal(response.EmojiReactions()[0].Type, protobuf.EmojiReaction_SAD)

	// Retract the emoji
	response, err = bob.SendEmojiReactionRetraction(context.Background(), emojiID)
	s.Require().NoError(err)
	s.Require().Len(response.EmojiReactions(), 1)
	s.Require().True(response.EmojiReactions()[0].Retracted)

	// Wait for the emoji to arrive to alice
	response, err = WaitOnMessengerResponse(
		alice,
		func(r *MessengerResponse) bool { return len(r.EmojiReactions()) == 1 },
		"no emoji",
	)
	s.Require().NoError(err)

	s.Require().Len(response.EmojiReactions(), 1)
	s.Require().Equal(response.EmojiReactions()[0].ID(), emojiID)
	s.Require().Equal(response.EmojiReactions()[0].Type, protobuf.EmojiReaction_SAD)
	s.Require().True(response.EmojiReactions()[0].Retracted)
}

func (s *MessengerEmojiSuite) TestEmojiPrivateGroup() {
	bob := s.m
	alice := s.newMessenger()
	_, err := alice.Start()
	s.Require().NoError(err)
	defer alice.Shutdown() // nolint: errcheck
	response, err := bob.CreateGroupChatWithMembers(context.Background(), "test", []string{})
	s.NoError(err)

	s.Require().NoError(makeMutualContact(bob, &alice.identity.PublicKey))

	chat := response.Chats()[0]
	members := []string{types.EncodeHex(crypto.FromECDSAPub(&alice.identity.PublicKey))}
	_, err = bob.AddMembersToGroupChat(context.Background(), chat.ID, members)
	s.NoError(err)

	// Retrieve their messages so that the chat is created
	_, err = WaitOnMessengerResponse(
		alice,
		func(r *MessengerResponse) bool { return len(r.Chats()) > 0 },
		"chat invitation not received",
	)
	s.Require().NoError(err)

	_, err = alice.ConfirmJoiningGroup(context.Background(), chat.ID)
	s.NoError(err)

	// Wait for the message to reach its destination
	_, err = WaitOnMessengerResponse(
		bob,
		func(r *MessengerResponse) bool { return len(r.Chats()) > 0 },
		"no joining group event received",
	)
	s.Require().NoError(err)

	inputMessage := buildTestMessage(*chat)
	_, err = bob.SendChatMessage(context.Background(), inputMessage)
	s.NoError(err)

	// Wait for the message to reach its destination
	response, err = WaitOnMessengerResponse(
		alice,
		func(r *MessengerResponse) bool { return len(r.Messages()) > 0 },
		"no message received",
	)
	s.Require().NoError(err)
	messageID := response.Messages()[0].ID

	_, err = bob.SendEmojiReaction(context.Background(), chat.ID, messageID, protobuf.EmojiReaction_SAD)
	s.Require().NoError(err)

	// Wait for the message to reach its destination
	_, err = WaitOnMessengerResponse(
		alice,
		func(r *MessengerResponse) bool { return len(r.EmojiReactions()) == 1 },
		"no emoji reaction received",
	)
	s.Require().NoError(err)
}

func (s *MessengerEmojiSuite) TestCompressedKeyReturnedWithEmoji() {
	emojiReaction := &EmojiReaction{}
	id, err := crypto.GenerateKey()
	s.Require().NoError(err)

	emojiReaction.From = common.PubkeyToHex(&id.PublicKey)
	emojiReaction.LocalChatID = testPublicChatID
	encodedReaction, err := json.Marshal(emojiReaction)
	s.Require().NoError(err)

	// Check that compressedKey and emojiHash exists
	s.Require().True(strings.Contains(string(encodedReaction), "compressedKey\":\"zQ"))
	s.Require().True(strings.Contains(string(encodedReaction), "emojiHash"))
}
