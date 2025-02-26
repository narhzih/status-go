package protocol

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"go.uber.org/zap"

	"github.com/status-im/status-go/eth-node/crypto"
	"github.com/status-im/status-go/eth-node/types"
	"github.com/status-im/status-go/protocol/common"
	"github.com/status-im/status-go/protocol/protobuf"
	"github.com/status-im/status-go/protocol/requests"
	"github.com/status-im/status-go/protocol/transport"
)

const outgoingMutualStateEventSentDefaultText = "You sent a contact request to @%s"
const outgoingMutualStateEventAcceptedDefaultText = "You accepted @%s's contact request"
const outgoingMutualStateEventRemovedDefaultText = "You removed @%s as a contact"
const incomingMutualStateEventSentDefaultText = "@%s sent you a contact request"
const incomingMutualStateEventAcceptedDefaultText = "@%s accepted your contact request"
const incomingMutualStateEventRemovedDefaultText = "@%s removed you as a contact"

func (m *Messenger) prepareMutualStateUpdateMessage(contactID string, updateType MutualStateUpdateType, clock uint64, timestamp uint64, outgoing bool) (*common.Message, error) {
	var text string
	var to string
	var from string
	var contentType protobuf.ChatMessage_ContentType
	if outgoing {
		to = contactID
		from = m.myHexIdentity()

		switch updateType {
		case MutualStateUpdateTypeSent:
			text = fmt.Sprintf(outgoingMutualStateEventSentDefaultText, contactID)
			contentType = protobuf.ChatMessage_SYSTEM_MESSAGE_MUTUAL_EVENT_SENT
		case MutualStateUpdateTypeAdded:
			text = fmt.Sprintf(outgoingMutualStateEventAcceptedDefaultText, contactID)
			contentType = protobuf.ChatMessage_SYSTEM_MESSAGE_MUTUAL_EVENT_ACCEPTED
		case MutualStateUpdateTypeRemoved:
			text = fmt.Sprintf(outgoingMutualStateEventRemovedDefaultText, contactID)
			contentType = protobuf.ChatMessage_SYSTEM_MESSAGE_MUTUAL_EVENT_REMOVED
		default:
			return nil, fmt.Errorf("unhandled outgoing MutualStateUpdateType = %d", updateType)
		}
	} else {
		to = m.myHexIdentity()
		from = contactID

		switch updateType {
		case MutualStateUpdateTypeSent:
			text = fmt.Sprintf(incomingMutualStateEventSentDefaultText, contactID)
			contentType = protobuf.ChatMessage_SYSTEM_MESSAGE_MUTUAL_EVENT_SENT
		case MutualStateUpdateTypeAdded:
			text = fmt.Sprintf(incomingMutualStateEventAcceptedDefaultText, contactID)
			contentType = protobuf.ChatMessage_SYSTEM_MESSAGE_MUTUAL_EVENT_ACCEPTED
		case MutualStateUpdateTypeRemoved:
			text = fmt.Sprintf(incomingMutualStateEventRemovedDefaultText, contactID)
			contentType = protobuf.ChatMessage_SYSTEM_MESSAGE_MUTUAL_EVENT_REMOVED
		default:
			return nil, fmt.Errorf("unhandled incoming MutualStateUpdateType = %d", updateType)
		}
	}

	message := &common.Message{
		ChatMessage: protobuf.ChatMessage{
			ChatId:      contactID,
			Text:        text,
			MessageType: protobuf.MessageType_ONE_TO_ONE,
			ContentType: contentType,
			Clock:       clock,
			Timestamp:   timestamp,
		},
		From:             from,
		WhisperTimestamp: timestamp,
		LocalChatID:      contactID,
		Seen:             outgoing,
		ID:               types.EncodeHex(crypto.Keccak256([]byte(fmt.Sprintf("%s%s%d%d", from, to, updateType, clock)))),
	}

	return message, nil
}

func (m *Messenger) acceptContactRequest(ctx context.Context, requestID string, syncing bool) (*MessengerResponse, error) {
	contactRequest, err := m.persistence.MessageByID(requestID)
	if err != nil {
		m.logger.Error("could not find contact request message", zap.Error(err))
		return nil, err
	}

	m.logger.Info("acceptContactRequest")

	response, err := m.addContact(ctx, contactRequest.From, "", "", "", contactRequest.ID, "", syncing, false, false)
	if err != nil {
		return nil, err
	}

	// Force activate chat
	chat, ok := m.allChats.Load(contactRequest.From)
	if !ok {
		publicKey, err := common.HexToPubkey(contactRequest.From)
		if err != nil {
			return nil, err
		}

		chat = OneToOneFromPublicKey(publicKey, m.getTimesource())
	}

	chat.Active = true
	if err := m.saveChat(chat); err != nil {
		return nil, err
	}
	response.AddChat(chat)

	return response, nil
}

func (m *Messenger) AcceptContactRequest(ctx context.Context, request *requests.AcceptContactRequest) (*MessengerResponse, error) {
	err := request.Validate()
	if err != nil {
		return nil, err
	}

	response, err := m.acceptContactRequest(ctx, request.ID.String(), false)
	if err != nil {
		return nil, err
	}

	err = m.syncContactRequestDecision(ctx, request.ID.String(), true, m.dispatchMessage)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (m *Messenger) declineContactRequest(requestID string, syncing bool) (*MessengerResponse, error) {
	m.logger.Info("declineContactRequest")
	contactRequest, err := m.persistence.MessageByID(requestID)
	if err != nil {
		return nil, err
	}

	contact, err := m.BuildContact(&requests.BuildContact{PublicKey: contactRequest.From})
	if err != nil {
		return nil, err
	}

	response := &MessengerResponse{}

	if !syncing {
		_, clock, err := m.getOneToOneAndNextClock(contact)
		if err != nil {
			return nil, err
		}

		contact.DismissContactRequest(clock)
		err = m.persistence.SaveContact(contact, nil)
		if err != nil {
			return nil, err
		}

		response.AddContact(contact)
	}
	contactRequest.ContactRequestState = common.ContactRequestStateDismissed

	err = m.persistence.SetContactRequestState(contactRequest.ID, contactRequest.ContactRequestState)
	if err != nil {
		return nil, err
	}

	// update notification with the correct status
	notification, err := m.persistence.GetActivityCenterNotificationByID(types.FromHex(contactRequest.ID))
	if err != nil {
		return nil, err
	}
	if notification != nil {
		notification.Name = contact.PrimaryName()
		notification.Message = contactRequest
		notification.Read = true
		notification.Dismissed = true
		notification.UpdatedAt = m.getCurrentTimeInMillis()

		err = m.addActivityCenterNotification(response, notification)
		if err != nil {
			m.logger.Error("failed to save notification", zap.Error(err))
			return nil, err
		}
	}
	response.AddMessage(contactRequest)
	return response, nil
}

func (m *Messenger) DeclineContactRequest(ctx context.Context, request *requests.DeclineContactRequest) (*MessengerResponse, error) {
	err := request.Validate()
	if err != nil {
		return nil, err
	}

	response, err := m.declineContactRequest(request.ID.String(), false)
	if err != nil {
		return nil, err
	}

	err = m.syncContactRequestDecision(ctx, request.ID.String(), false, m.dispatchMessage)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (m *Messenger) SendContactRequest(ctx context.Context, request *requests.SendContactRequest) (*MessengerResponse, error) {
	err := request.Validate()
	if err != nil {
		return nil, err
	}

	chatID, err := request.HexID()
	if err != nil {
		return nil, err
	}

	return m.addContact(
		ctx,
		chatID,
		"",
		"",
		"",
		"",
		request.Message,
		false,
		false,
		true,
	)
}

func (m *Messenger) updateAcceptedContactRequest(response *MessengerResponse, contactRequestID string) (*MessengerResponse, error) {

	m.logger.Debug("updateAcceptedContactRequest", zap.String("contactRequestID", contactRequestID))

	contactRequest, err := m.persistence.MessageByID(contactRequestID)
	if err != nil {
		m.logger.Error("contact request not found", zap.String("contactRequestID", contactRequestID), zap.Error(err))
		return nil, err
	}

	contactRequest.ContactRequestState = common.ContactRequestStateAccepted

	err = m.persistence.SetContactRequestState(contactRequest.ID, contactRequest.ContactRequestState)
	if err != nil {
		return nil, err
	}

	contact, ok := m.allContacts.Load(contactRequest.From)
	if !ok {
		m.logger.Error("failed to update contact request: contact not found", zap.String("contact id", contactRequest.From))
		return nil, errors.New("failed to update contact request: contact not found")
	}

	chat, ok := m.allChats.Load(contact.ID)
	if !ok {
		return nil, errors.New("no chat found for accepted contact request")
	}

	notification, err := m.persistence.GetActivityCenterNotificationByID(types.FromHex(contactRequest.ID))
	if err != nil {
		return nil, err
	}

	clock, _ := chat.NextClockAndTimestamp(m.transport)
	contact.AcceptContactRequest(clock)

	acceptContactRequest := &protobuf.AcceptContactRequest{
		Id:    contactRequest.ID,
		Clock: clock,
	}
	encodedMessage, err := proto.Marshal(acceptContactRequest)
	if err != nil {
		return nil, err
	}

	_, err = m.dispatchMessage(context.Background(), common.RawMessage{
		LocalChatID:         contactRequest.From,
		Payload:             encodedMessage,
		MessageType:         protobuf.ApplicationMetadataMessage_ACCEPT_CONTACT_REQUEST,
		ResendAutomatically: true,
	})
	if err != nil {
		return nil, err
	}

	if response == nil {
		response = &MessengerResponse{}
	}

	if notification != nil {
		notification.Name = contact.PrimaryName()
		notification.Message = contactRequest
		notification.Read = true
		notification.Accepted = true
		notification.UpdatedAt = m.getCurrentTimeInMillis()

		err = m.addActivityCenterNotification(response, notification)
		if err != nil {
			m.logger.Error("failed to save notification", zap.Error(err))
			return nil, err
		}
	}

	response.AddMessage(contactRequest)
	response.AddContact(contact)

	// Add mutual state update message for incoming contact request
	clock, timestamp := chat.NextClockAndTimestamp(m.transport)
	updateMessage, err := m.prepareMutualStateUpdateMessage(contact.ID, MutualStateUpdateTypeAdded, clock, timestamp, true)
	if err != nil {
		return nil, err
	}

	m.prepareMessage(updateMessage, m.httpServer)
	err = m.persistence.SaveMessages([]*common.Message{updateMessage})
	if err != nil {
		return nil, err
	}
	response.AddMessage(updateMessage)
	response.AddChat(chat)

	return response, nil
}

func (m *Messenger) addContact(ctx context.Context, pubKey, ensName, nickname, displayName, contactRequestID string, contactRequestText string, syncing bool, sendContactUpdate bool, createOutgoingContactRequestNotification bool) (*MessengerResponse, error) {
	contact, err := m.BuildContact(&requests.BuildContact{PublicKey: pubKey})
	if err != nil {
		return nil, err
	}

	response := &MessengerResponse{}

	chat, clock, err := m.getOneToOneAndNextClock(contact)
	if err != nil {
		return nil, err
	}

	if ensName != "" {
		err := m.ensVerifier.ENSVerified(pubKey, ensName, clock)
		if err != nil {
			return nil, err
		}
	}
	if err := m.addENSNameToContact(contact); err != nil {
		return nil, err
	}

	if len(nickname) != 0 {
		contact.LocalNickname = nickname
	}

	if len(displayName) != 0 {
		contact.DisplayName = displayName
	}

	contact.LastUpdatedLocally = clock
	contact.ContactRequestSent(clock)

	if !syncing {
		// We sync the contact with the other devices
		err := m.syncContact(context.Background(), contact, m.dispatchMessage)
		if err != nil {
			return nil, err
		}
	}

	err = m.persistence.SaveContact(contact, nil)
	if err != nil {
		return nil, err
	}

	// TODO(samyoul) remove storing of an updated reference pointer?
	m.allContacts.Store(contact.ID, contact)

	// And we re-register for push notications
	err = m.reregisterForPushNotifications()
	if err != nil {
		return nil, err
	}

	// Reset last published time for ChatIdentity so new contact can receive data
	err = m.resetLastPublishedTimeForChatIdentity()
	if err != nil {
		return nil, err
	}

	// Create the corresponding chat
	profileChat := m.buildProfileChat(contact.ID)

	_, err = m.Join(profileChat)
	if err != nil {
		return nil, err
	}

	if err := m.saveChat(profileChat); err != nil {
		return nil, err
	}

	publicKey, err := contact.PublicKey()
	if err != nil {
		return nil, err
	}

	// Fetch contact code
	_, err = m.scheduleSyncFiltersForContact(publicKey)
	if err != nil {
		return nil, err
	}

	// Get ENS name of a current user
	ensName, err = m.settings.ENSName()
	if err != nil {
		return nil, err
	}

	// Get display name of a current user
	displayName, err = m.settings.DisplayName()
	if err != nil {
		return nil, err
	}

	if sendContactUpdate {
		response, err = m.sendContactUpdate(context.Background(), pubKey, displayName, ensName, "", m.dispatchMessage)
		if err != nil {
			return nil, err
		}
	}

	if len(contactRequestID) != 0 {
		updatedResponse, err := m.updateAcceptedContactRequest(response, contactRequestID)
		if err != nil {
			return nil, err
		}
		err = response.Merge(updatedResponse)
		if err != nil {
			return nil, err
		}
	}

	// Sends a standalone ChatIdentity message
	err = m.handleStandaloneChatIdentity(chat)
	if err != nil {
		return nil, err
	}

	// Add chat
	response.AddChat(profileChat)

	_, err = m.transport.InitFilters([]string{profileChat.ID}, []*ecdsa.PublicKey{publicKey})
	if err != nil {
		return nil, err
	}

	// Publish contact code
	err = m.publishContactCode()
	if err != nil {
		return nil, err
	}

	// Add mutual state update message for outgoing contact request
	if len(contactRequestID) == 0 {
		clock, timestamp := chat.NextClockAndTimestamp(m.transport)
		updateMessage, err := m.prepareMutualStateUpdateMessage(contact.ID, MutualStateUpdateTypeSent, clock, timestamp, true)
		if err != nil {
			return nil, err
		}

		m.prepareMessage(updateMessage, m.httpServer)
		err = m.persistence.SaveMessages([]*common.Message{updateMessage})
		if err != nil {
			return nil, err
		}
		response.AddMessage(updateMessage)
		response.AddChat(chat)
	}

	// Add outgoing contact request notification
	if createOutgoingContactRequestNotification {
		clock, timestamp := chat.NextClockAndTimestamp(m.transport)
		contactRequest, err := m.generateContactRequest(clock, timestamp, contact, contactRequestText, true)
		if err != nil {
			return nil, err
		}

		// Send contact request as a plain chat message
		messageResponse, err := m.sendChatMessage(ctx, contactRequest)
		if err != nil {
			return nil, err
		}

		err = response.Merge(messageResponse)
		if err != nil {
			return nil, err
		}

		notification := m.generateOutgoingContactRequestNotification(contact, contactRequest)
		err = m.addActivityCenterNotification(response, notification)
		if err != nil {
			return nil, err
		}
	}

	// Add contact
	response.AddContact(contact)
	return response, nil
}

func (m *Messenger) generateContactRequest(clock uint64, timestamp uint64, contact *Contact, text string, outgoing bool) (*common.Message, error) {
	if contact == nil {
		return nil, errors.New("contact cannot be nil")
	}

	contactRequest := &common.Message{}
	contactRequest.ChatId = contact.ID
	contactRequest.WhisperTimestamp = timestamp
	contactRequest.Seen = true
	contactRequest.Text = text
	if outgoing {
		contactRequest.From = m.myHexIdentity()
	} else {
		contactRequest.From = contact.ID
	}
	contactRequest.LocalChatID = contact.ID
	contactRequest.ContentType = protobuf.ChatMessage_CONTACT_REQUEST
	contactRequest.Clock = clock
	if contact.mutual() {
		contactRequest.ContactRequestState = common.ContactRequestStateAccepted
	} else {
		contactRequest.ContactRequestState = common.ContactRequestStatePending
	}
	err := contactRequest.PrepareContent(common.PubkeyToHex(&m.identity.PublicKey))
	return contactRequest, err
}

func (m *Messenger) generateOutgoingContactRequestNotification(contact *Contact, contactRequest *common.Message) *ActivityCenterNotification {
	return &ActivityCenterNotification{
		ID:        types.FromHex(contactRequest.ID),
		Type:      ActivityCenterNotificationTypeContactRequest,
		Name:      contact.PrimaryName(),
		Author:    m.myHexIdentity(),
		Message:   contactRequest,
		Timestamp: m.getTimesource().GetCurrentTime(),
		ChatID:    contact.ID,
		Read: contactRequest.ContactRequestState == common.ContactRequestStateAccepted ||
			contactRequest.ContactRequestState == common.ContactRequestStateDismissed ||
			contactRequest.ContactRequestState == common.ContactRequestStatePending,
		Accepted:  contactRequest.ContactRequestState == common.ContactRequestStateAccepted,
		Dismissed: contactRequest.ContactRequestState == common.ContactRequestStateDismissed,
		UpdatedAt: m.getCurrentTimeInMillis(),
	}
}

func (m *Messenger) AddContact(ctx context.Context, request *requests.AddContact) (*MessengerResponse, error) {
	err := request.Validate()
	if err != nil {
		return nil, err
	}

	id, err := request.HexID()
	if err != nil {
		return nil, err
	}

	return m.addContact(
		ctx,
		id,
		request.ENSName,
		request.Nickname,
		request.DisplayName,
		"",
		defaultContactRequestText(),
		false,
		true,
		true,
	)
}

func (m *Messenger) resetLastPublishedTimeForChatIdentity() error {
	// Reset last published time for ChatIdentity so new contact can receive data
	contactCodeTopic := transport.ContactCodeTopic(&m.identity.PublicKey)
	m.logger.Debug("contact state changed ResetWhenChatIdentityLastPublished")
	return m.persistence.ResetWhenChatIdentityLastPublished(contactCodeTopic)
}

func (m *Messenger) removeContact(ctx context.Context, response *MessengerResponse, pubKey string, sync bool) error {
	contact, ok := m.allContacts.Load(pubKey)
	if !ok {
		return ErrContactNotFound
	}

	// System message for mutual state update
	chat, clock, err := m.getOneToOneAndNextClock(contact)
	if err != nil {
		return err
	}
	timestamp := m.getTimesource().GetCurrentTime()
	updateMessage, err := m.prepareMutualStateUpdateMessage(contact.ID, MutualStateUpdateTypeRemoved, clock, timestamp, true)
	if err != nil {
		return err
	}

	m.prepareMessage(updateMessage, m.httpServer)
	err = m.persistence.SaveMessages([]*common.Message{updateMessage})
	if err != nil {
		return err
	}
	response.AddMessage(updateMessage)
	response.AddChat(chat)

	// Next we retract a contact request
	contact.RetractContactRequest(clock)
	contact.LastUpdatedLocally = m.getTimesource().GetCurrentTime()

	err = m.persistence.SaveContact(contact, nil)
	if err != nil {
		return err
	}

	if sync {
		err = m.syncContact(context.Background(), contact, m.dispatchMessage)
		if err != nil {
			return err
		}
	}

	// TODO(samyoul) remove storing of an updated reference pointer?
	m.allContacts.Store(contact.ID, contact)

	// And we re-register for push notications
	err = m.reregisterForPushNotifications()
	if err != nil {
		return err
	}

	// Create the corresponding profile chat
	profileChatID := buildProfileChatID(contact.ID)
	_, ok = m.allChats.Load(profileChatID)

	if ok {
		chatResponse, err := m.deactivateChat(profileChatID, 0, false, true)
		if err != nil {
			return err
		}
		err = response.Merge(chatResponse)
		if err != nil {
			return err
		}
	}

	response.Contacts = []*Contact{contact}
	return nil
}

func (m *Messenger) RemoveContact(ctx context.Context, pubKey string) (*MessengerResponse, error) {
	response := new(MessengerResponse)

	err := m.removeContact(ctx, response, pubKey, true)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (m *Messenger) updateContactImagesURL(contact *Contact) error {
	if m.httpServer != nil {
		for k, v := range contact.Images {
			publicKey, err := contact.PublicKey()
			if err != nil {
				return err
			}
			v.LocalURL = m.httpServer.MakeContactImageURL(common.PubkeyToHex(publicKey), k)
			contact.Images[k] = v
		}
	}
	return nil
}

func (m *Messenger) Contacts() []*Contact {
	var contacts []*Contact
	m.allContacts.Range(func(contactID string, contact *Contact) (shouldContinue bool) {
		contacts = append(contacts, contact)
		return true
	})
	return contacts
}

func (m *Messenger) AddedContacts() []*Contact {
	var contacts []*Contact
	m.allContacts.Range(func(contactID string, contact *Contact) (shouldContinue bool) {
		if contact.added() {
			contacts = append(contacts, contact)
		}
		return true
	})
	return contacts
}

func (m *Messenger) MutualContacts() []*Contact {
	var contacts []*Contact
	m.allContacts.Range(func(contactID string, contact *Contact) (shouldContinue bool) {
		if contact.mutual() {
			contacts = append(contacts, contact)
		}
		return true
	})
	return contacts
}

func (m *Messenger) BlockedContacts() []*Contact {
	var contacts []*Contact
	m.allContacts.Range(func(contactID string, contact *Contact) (shouldContinue bool) {
		if contact.Blocked {
			contacts = append(contacts, contact)
		}
		return true
	})
	return contacts
}

// GetContactByID assumes pubKey includes 0x prefix
func (m *Messenger) GetContactByID(pubKey string) *Contact {
	contact, _ := m.allContacts.Load(pubKey)
	return contact
}

func (m *Messenger) SetContactLocalNickname(request *requests.SetContactLocalNickname) (*MessengerResponse, error) {

	if err := request.Validate(); err != nil {
		return nil, err
	}

	pubKey := request.ID.String()
	nickname := request.Nickname

	contact, err := m.BuildContact(&requests.BuildContact{PublicKey: pubKey})
	if err != nil {
		return nil, err
	}

	if err := m.addENSNameToContact(contact); err != nil {
		return nil, err
	}

	clock := m.getTimesource().GetCurrentTime()
	contact.LocalNickname = nickname
	contact.LastUpdatedLocally = clock

	err = m.persistence.SaveContact(contact, nil)
	if err != nil {
		return nil, err
	}

	m.allContacts.Store(contact.ID, contact)

	response := &MessengerResponse{}
	response.Contacts = []*Contact{contact}

	err = m.syncContact(context.Background(), contact, m.dispatchMessage)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (m *Messenger) blockContact(response *MessengerResponse, contactID string, isDesktopFunc bool) (*Contact, []*Chat, error) {
	contact, err := m.BuildContact(&requests.BuildContact{PublicKey: contactID})
	if err != nil {
		return nil, nil, err
	}

	_, clock, err := m.getOneToOneAndNextClock(contact)
	if err != nil {
		return nil, nil, err
	}

	contact.Block(clock)

	contact.LastUpdatedLocally = m.getTimesource().GetCurrentTime()

	chats, err := m.persistence.BlockContact(contact, isDesktopFunc)
	if err != nil {
		return nil, nil, err
	}

	err = m.sendRetractContactRequest(contact)
	if err != nil {
		return nil, nil, err
	}

	m.allContacts.Store(contact.ID, contact)
	for _, chat := range chats {
		m.allChats.Store(chat.ID, chat)
	}

	if !isDesktopFunc {
		m.allChats.Delete(contact.ID)
		m.allChats.Delete(buildProfileChatID(contact.ID))
	}

	err = m.syncContact(context.Background(), contact, m.dispatchMessage)
	if err != nil {
		return nil, nil, err
	}

	// re-register for push notifications
	err = m.reregisterForPushNotifications()
	if err != nil {
		return nil, nil, err
	}

	// We remove anything that's related to this contact request
	notifications, err := m.persistence.DeleteChatContactRequestActivityCenterNotifications(contact.ID, m.getCurrentTimeInMillis())
	if err != nil {
		return nil, nil, err
	}

	err = m.syncActivityCenterNotifications(notifications)
	if err != nil {
		m.logger.Error("BlockContact, error syncing activity center notifications", zap.Error(err))
		return nil, nil, err
	}

	return contact, chats, nil
}

func (m *Messenger) BlockContact(contactID string) (*MessengerResponse, error) {
	response := &MessengerResponse{}

	contact, chats, err := m.blockContact(response, contactID, false)
	if err != nil {
		return nil, err
	}
	response.AddChats(chats)
	response.AddContact(contact)

	response, err = m.DeclineAllPendingGroupInvitesFromUser(response, contactID)
	if err != nil {
		return nil, err
	}

	notifications, err := m.persistence.DismissAllActivityCenterNotificationsFromUser(contactID, m.getCurrentTimeInMillis())
	if err != nil {
		return nil, err
	}

	err = m.syncActivityCenterNotifications(notifications)
	if err != nil {
		m.logger.Error("BlockContact, error syncing activity center notifications", zap.Error(err))
		return nil, err
	}

	return response, nil
}

// The same function as the one above.
func (m *Messenger) BlockContactDesktop(contactID string) (*MessengerResponse, error) {
	response := &MessengerResponse{}

	contact, chats, err := m.blockContact(response, contactID, true)
	if err != nil {
		return nil, err
	}
	response.AddChats(chats)
	response.AddContact(contact)

	response, err = m.DeclineAllPendingGroupInvitesFromUser(response, contactID)
	if err != nil {
		return nil, err
	}

	notifications, err := m.persistence.DismissAllActivityCenterNotificationsFromUser(contactID, m.getCurrentTimeInMillis())
	if err != nil {
		return nil, err
	}

	err = m.syncActivityCenterNotifications(notifications)
	if err != nil {
		m.logger.Error("BlockContactDesktop, error syncing activity center notifications", zap.Error(err))
		return nil, err
	}

	return response, nil
}

func (m *Messenger) UnblockContact(contactID string) (*MessengerResponse, error) {
	response := &MessengerResponse{}
	contact, ok := m.allContacts.Load(contactID)
	if !ok || !contact.Blocked {
		return response, nil
	}

	_, clock, err := m.getOneToOneAndNextClock(contact)
	if err != nil {
		return nil, err
	}

	contact.Unblock(clock)

	contact.LastUpdatedLocally = m.getTimesource().GetCurrentTime()

	err = m.persistence.SaveContact(contact, nil)
	if err != nil {
		return nil, err
	}

	m.allContacts.Store(contact.ID, contact)

	response.AddContact(contact)

	err = m.syncContact(context.Background(), contact, m.dispatchMessage)
	if err != nil {
		return nil, err
	}

	// re-register for push notifications
	err = m.reregisterForPushNotifications()
	if err != nil {
		return nil, err
	}

	return response, nil
}

// Send contact updates to all contacts added by us
func (m *Messenger) SendContactUpdates(ctx context.Context, ensName, profileImage string) (err error) {
	myID := contactIDFromPublicKey(&m.identity.PublicKey)

	displayName, err := m.settings.DisplayName()
	if err != nil {
		return err
	}

	if _, err = m.sendContactUpdate(ctx, myID, displayName, ensName, profileImage, m.dispatchMessage); err != nil {
		return err
	}

	// TODO: This should not be sending paired messages, as we do it above
	m.allContacts.Range(func(contactID string, contact *Contact) (shouldContinue bool) {
		if contact.added() {
			if _, err = m.sendContactUpdate(ctx, contact.ID, displayName, ensName, profileImage, m.dispatchMessage); err != nil {
				return false
			}
		}
		return true
	})
	return err
}

// NOTE: this endpoint does not add the contact, the reason being is that currently
// that's left as a responsibility to the client, which will call both `SendContactUpdate`
// and `SaveContact` with the correct system tag.
// Ideally we have a single endpoint that does both, but probably best to bring `ENS` name
// on the messenger first.

// SendContactUpdate sends a contact update to a user and adds the user to contacts
func (m *Messenger) SendContactUpdate(ctx context.Context, chatID, ensName, profileImage string) (*MessengerResponse, error) {
	displayName, err := m.settings.DisplayName()
	if err != nil {
		return nil, err
	}

	return m.sendContactUpdate(ctx, chatID, displayName, ensName, profileImage, m.dispatchMessage)
}

func (m *Messenger) sendContactUpdate(ctx context.Context, chatID, displayName, ensName, profileImage string, rawMessageHandler RawMessageHandler) (*MessengerResponse, error) {
	var response MessengerResponse

	contact, ok := m.allContacts.Load(chatID)
	if !ok || !contact.added() {
		return nil, nil
	}

	chat, clock, err := m.getOneToOneAndNextClock(contact)
	if err != nil {
		return nil, err
	}

	contactUpdate := &protobuf.ContactUpdate{
		Clock:                         clock,
		DisplayName:                   displayName,
		EnsName:                       ensName,
		ProfileImage:                  profileImage,
		ContactRequestClock:           contact.ContactRequestLocalClock,
		ContactRequestPropagatedState: contact.ContactRequestPropagatedState(),
		PublicKey:                     contact.ID,
	}
	encodedMessage, err := proto.Marshal(contactUpdate)
	if err != nil {
		return nil, err
	}

	rawMessage := common.RawMessage{
		LocalChatID:         chatID,
		Payload:             encodedMessage,
		MessageType:         protobuf.ApplicationMetadataMessage_CONTACT_UPDATE,
		ResendAutomatically: true,
	}

	_, err = rawMessageHandler(ctx, rawMessage)
	if err != nil {
		return nil, err
	}

	response.Contacts = []*Contact{contact}
	response.AddChat(chat)

	chat.LastClockValue = clock
	err = m.saveChat(chat)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (m *Messenger) addENSNameToContact(contact *Contact) error {

	// Check if there's already a verified record
	ensRecord, err := m.ensVerifier.GetVerifiedRecord(contact.ID)
	if err != nil {
		return err
	}
	if ensRecord == nil {
		return nil
	}

	contact.EnsName = ensRecord.Name
	contact.ENSVerified = true

	return nil
}

func (m *Messenger) RetractContactRequest(request *requests.RetractContactRequest) (*MessengerResponse, error) {
	err := request.Validate()
	if err != nil {
		return nil, err
	}
	contact, ok := m.allContacts.Load(request.ID.String())
	if !ok {
		return nil, errors.New("contact not found")
	}
	response := &MessengerResponse{}
	err = m.removeContact(context.Background(), response, contact.ID, true)
	if err != nil {
		return nil, err
	}

	err = m.sendRetractContactRequest(contact)
	if err != nil {
		return nil, err
	}

	return response, err
}

// Send message to remote account to remove our contact from their end.
func (m *Messenger) sendRetractContactRequest(contact *Contact) error {
	_, clock, err := m.getOneToOneAndNextClock(contact)
	if err != nil {
		return err
	}
	retractContactRequest := &protobuf.RetractContactRequest{
		Clock: clock,
	}

	encodedMessage, err := proto.Marshal(retractContactRequest)
	if err != nil {
		return err
	}

	_, err = m.dispatchMessage(context.Background(), common.RawMessage{
		LocalChatID:         contact.ID,
		Payload:             encodedMessage,
		MessageType:         protobuf.ApplicationMetadataMessage_RETRACT_CONTACT_REQUEST,
		ResendAutomatically: true,
	})
	if err != nil {
		return err
	}

	return err
}

func (m *Messenger) AcceptLatestContactRequestForContact(ctx context.Context, request *requests.AcceptLatestContactRequestForContact) (*MessengerResponse, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}

	contactRequestID, err := m.persistence.LatestPendingContactRequestIDForContact(request.ID.String())
	if err != nil {
		return nil, err
	}

	return m.AcceptContactRequest(ctx, &requests.AcceptContactRequest{ID: types.Hex2Bytes(contactRequestID)})
}

func (m *Messenger) DismissLatestContactRequestForContact(ctx context.Context, request *requests.DismissLatestContactRequestForContact) (*MessengerResponse, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}

	contactRequestID, err := m.persistence.LatestPendingContactRequestIDForContact(request.ID.String())
	if err != nil {
		return nil, err
	}

	return m.DeclineContactRequest(ctx, &requests.DeclineContactRequest{ID: types.Hex2Bytes(contactRequestID)})
}

func (m *Messenger) PendingContactRequests(cursor string, limit int) ([]*common.Message, string, error) {
	return m.persistence.PendingContactRequests(cursor, limit)
}

func defaultContactRequestID(contactID string) string {
	return "0x" + types.Bytes2Hex(append(types.Hex2Bytes(contactID), 0x20))
}

func defaultContactRequestText() string {
	return "Please add me to your contacts"
}

func (m *Messenger) BuildContact(request *requests.BuildContact) (*Contact, error) {
	contact, ok := m.allContacts.Load(request.PublicKey)
	if !ok {
		var err error
		contact, err = buildContactFromPkString(request.PublicKey)
		if err != nil {
			return nil, err
		}

		if request.ENSName != "" {
			contact.ENSVerified = true
			contact.EnsName = request.ENSName
		}
	}

	// Schedule sync filter to fetch information about the contact

	publicKey, err := contact.PublicKey()
	if err != nil {
		return nil, err
	}

	_, err = m.scheduleSyncFiltersForContact(publicKey)
	if err != nil {
		return nil, err
	}

	return contact, nil
}

func (m *Messenger) scheduleSyncFiltersForContact(publicKey *ecdsa.PublicKey) (*transport.Filter, error) {
	filter, err := m.transport.JoinPrivate(publicKey)
	if err != nil {
		return nil, err
	}
	_, err = m.scheduleSyncFilters([]*transport.Filter{filter})
	if err != nil {
		return filter, err
	}
	return filter, nil
}

func (m *Messenger) RequestContactInfoFromMailserver(pubkey string, waitForResponse bool) (*Contact, error) {

	err := m.requestContactInfoFromMailserver(pubkey)

	if err != nil {
		return nil, err
	}

	if !waitForResponse {
		return nil, nil
	}

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var contact *Contact
	fetching := true

	for fetching {
		select {
		case <-time.After(200 * time.Millisecond):
			var ok bool
			contact, ok = m.allContacts.Load(pubkey)

			if ok && contact != nil && contact.DisplayName != "" {
				fetching = false
				m.logger.Info("contact info received", zap.String("pubkey", contact.ID))
			}

		case <-ctx.Done():
			fetching = false
		}
	}

	m.forgetContactInfoRequest(pubkey)

	return contact, nil
}

func (m *Messenger) requestContactInfoFromMailserver(pubkey string) error {

	m.requestedContactsLock.Lock()
	defer m.requestedContactsLock.Unlock()

	if _, ok := m.requestedContacts[pubkey]; ok {
		return nil
	}

	m.logger.Debug("requesting contact info from mailserver", zap.String("publicKey", pubkey))

	c, err := buildContactFromPkString(pubkey)

	if err != nil {
		return err
	}

	publicKey, err := c.PublicKey()

	if err != nil {
		return err
	}

	var filter *transport.Filter
	filter, err = m.scheduleSyncFiltersForContact(publicKey)

	if err != nil {
		return err
	}

	m.requestedContacts[pubkey] = filter

	return nil
}

func (m *Messenger) forgetContactInfoRequest(publicKey string) {

	m.requestedContactsLock.Lock()
	defer m.requestedContactsLock.Unlock()

	filter, ok := m.requestedContacts[publicKey]
	if !ok {
		return
	}

	m.logger.Debug("forgetting contact info request", zap.String("publicKey", publicKey))

	err := m.transport.RemoveFilters([]*transport.Filter{filter})

	if err != nil {
		m.logger.Warn("failed to remove filter", zap.Error(err))
	}

	delete(m.requestedContacts, publicKey)
}
