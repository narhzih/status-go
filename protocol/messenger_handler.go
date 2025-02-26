package protocol

import (
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/status-im/status-go/signal"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/google/uuid"

	"github.com/status-im/status-go/eth-node/crypto"
	"github.com/status-im/status-go/eth-node/types"
	"github.com/status-im/status-go/images"
	"github.com/status-im/status-go/multiaccounts/accounts"
	multiaccountscommon "github.com/status-im/status-go/multiaccounts/common"
	"github.com/status-im/status-go/multiaccounts/settings"
	"github.com/status-im/status-go/protocol/common"
	"github.com/status-im/status-go/protocol/communities"
	"github.com/status-im/status-go/protocol/encryption/multidevice"
	"github.com/status-im/status-go/protocol/identity"
	"github.com/status-im/status-go/protocol/protobuf"
	"github.com/status-im/status-go/protocol/requests"
	"github.com/status-im/status-go/protocol/transport"
	v1protocol "github.com/status-im/status-go/protocol/v1"
	"github.com/status-im/status-go/protocol/verification"
)

const (
	transactionRequestDeclinedMessage           = "Transaction request declined"
	requestAddressForTransactionAcceptedMessage = "Request address for transaction accepted"
	requestAddressForTransactionDeclinedMessage = "Request address for transaction declined"
)

var (
	ErrMessageNotAllowed                     = errors.New("message from a non-contact")
	ErrMessageForWrongChatType               = errors.New("message for the wrong chat type")
	ErrNotWatchOnlyAccount                   = errors.New("an account is not a watch only account")
	ErrWalletAccountNotSupportedForMobileApp = errors.New("handling account is not supported for mobile app")
	ErrTryingToApplyOldWalletAccountsOrder   = errors.New("trying to apply old wallet accounts order")
	ErrTryingToStoreOldWalletAccount         = errors.New("trying to store an old wallet account")
	ErrTryingToStoreOldKeypair               = errors.New("trying to store an old keypair")
	ErrSomeFieldsMissingForWalletAccount     = errors.New("some fields are missing for wallet account")
	ErrTryingToRemoveUnexistingWalletAccount = errors.New("trying to remove an unexisting wallet account")
	ErrUnknownKeypairForWalletAccount        = errors.New("keypair is not known for the wallet account")
	ErrInvalidCommunityID                    = errors.New("invalid community id")
)

// HandleMembershipUpdate updates a Chat instance according to the membership updates.
// It retrieves chat, if exists, and merges membership updates from the message.
// Finally, the Chat is updated with the new group events.
func (m *Messenger) HandleMembershipUpdate(messageState *ReceivedMessageState, chat *Chat, rawMembershipUpdate protobuf.MembershipUpdateMessage, translations *systemMessageTranslationsMap) error {
	var group *v1protocol.Group
	var err error

	logger := m.logger.With(zap.String("site", "HandleMembershipUpdate"))

	message, err := v1protocol.MembershipUpdateMessageFromProtobuf(&rawMembershipUpdate)
	if err != nil {
		return err

	}

	if err := ValidateMembershipUpdateMessage(message, messageState.Timesource.GetCurrentTime()); err != nil {
		logger.Warn("failed to validate message", zap.Error(err))
		return err
	}

	senderID := messageState.CurrentMessageState.Contact.ID
	allowed, err := m.isMessageAllowedFrom(senderID, chat)
	if err != nil {
		return err
	}

	if !allowed {
		return ErrMessageNotAllowed
	}

	//if chat.InvitationAdmin exists means we are waiting for invitation request approvement, and in that case
	//we need to create a new chat instance like we don't have a chat and just use a regular invitation flow
	waitingForApproval := chat != nil && len(chat.InvitationAdmin) > 0
	ourKey := contactIDFromPublicKey(&m.identity.PublicKey)
	isActive := messageState.CurrentMessageState.Contact.added() || messageState.CurrentMessageState.Contact.ID == ourKey || waitingForApproval
	showPushNotification := isActive && messageState.CurrentMessageState.Contact.ID != ourKey

	// wasUserAdded indicates whether the user has been added to the group with this update
	wasUserAdded := false
	if chat == nil || waitingForApproval {
		if len(message.Events) == 0 {
			return errors.New("can't create new group chat without events")
		}

		//approve invitations
		if waitingForApproval {

			groupChatInvitation := &GroupChatInvitation{
				GroupChatInvitation: protobuf.GroupChatInvitation{
					ChatId: message.ChatID,
				},
				From: types.EncodeHex(crypto.FromECDSAPub(&m.identity.PublicKey)),
			}

			groupChatInvitation, err = m.persistence.InvitationByID(groupChatInvitation.ID())
			if err != nil && err != common.ErrRecordNotFound {
				return err
			}
			if groupChatInvitation != nil {
				groupChatInvitation.State = protobuf.GroupChatInvitation_APPROVED

				err := m.persistence.SaveInvitation(groupChatInvitation)
				if err != nil {
					return err
				}
				messageState.GroupChatInvitations[groupChatInvitation.ID()] = groupChatInvitation
			}
		}

		group, err = v1protocol.NewGroupWithEvents(message.ChatID, message.Events)
		if err != nil {
			return err
		}

		// A new chat must contain us
		if !group.IsMember(ourKey) {
			return errors.New("can't create a new group chat without us being a member")
		}
		// A new chat always adds us
		wasUserAdded = true
		newChat := CreateGroupChat(messageState.Timesource)
		// We set group chat inactive and create a notification instead
		// unless is coming from us or a contact or were waiting for approval.
		// Also, as message MEMBER_JOINED may come from member(not creator, not our contact)
		// reach earlier than CHAT_CREATED from creator, we need check if creator is our contact
		newChat.Active = isActive || m.checkIfCreatorIsOurContact(group)
		newChat.ReceivedInvitationAdmin = senderID
		chat = &newChat

		chat.updateChatFromGroupMembershipChanges(group)

		if err != nil {
			return errors.Wrap(err, "failed to get group creator")
		}

	} else {
		existingGroup, err := newProtocolGroupFromChat(chat)
		if err != nil {
			return errors.Wrap(err, "failed to create a Group from Chat")
		}
		updateGroup, err := v1protocol.NewGroupWithEvents(message.ChatID, message.Events)
		if err != nil {
			return errors.Wrap(err, "invalid membership update")
		}
		merged := v1protocol.MergeMembershipUpdateEvents(existingGroup.Events(), updateGroup.Events())
		group, err = v1protocol.NewGroupWithEvents(chat.ID, merged)
		if err != nil {
			return errors.Wrap(err, "failed to create a group with new membership updates")
		}
		chat.updateChatFromGroupMembershipChanges(group)

		// Reactivate deleted group chat on re-invite from contact
		chat.Active = chat.Active || (isActive && group.IsMember(ourKey))

		wasUserAdded = !existingGroup.IsMember(ourKey) && group.IsMember(ourKey)

		// Show push notifications when our key is added to members list and chat is Active
		showPushNotification = showPushNotification && wasUserAdded
	}
	maxClockVal := uint64(0)
	for _, event := range group.Events() {
		if event.ClockValue > maxClockVal {
			maxClockVal = event.ClockValue
		}
	}

	if chat.LastClockValue < maxClockVal {
		chat.LastClockValue = maxClockVal
	}

	// Only create a message notification when the user is added, not when removed
	if !chat.Active && wasUserAdded {
		chat.Highlight = true
		m.createMessageNotification(chat, messageState, chat.LastMessage)
	}

	profilePicturesVisibility, err := m.settings.GetProfilePicturesVisibility()
	if err != nil {
		return errors.Wrap(err, "failed to get profilePicturesVisibility setting")
	}

	if showPushNotification {
		// chat is highlighted for new group invites or group re-invites
		chat.Highlight = true
		messageState.Response.AddNotification(NewPrivateGroupInviteNotification(chat.ID, chat, messageState.CurrentMessageState.Contact, profilePicturesVisibility))
	}

	systemMessages := buildSystemMessages(message.Events, translations)

	for _, message := range systemMessages {
		messageID := message.ID
		exists, err := m.messageExists(messageID, messageState.ExistingMessagesMap)
		if err != nil {
			m.logger.Warn("failed to check message exists", zap.Error(err))
		}
		if exists {
			continue
		}
		messageState.Response.AddMessage(message)
	}

	messageState.Response.AddChat(chat)
	// Store in chats map as it might be a new one
	messageState.AllChats.Store(chat.ID, chat)

	// explicit join has been removed, mimic auto-join for backward compatibility
	// no all cases are covered, e.g. if added to a group by non-contact
	autoJoin := chat.Active && wasUserAdded
	if autoJoin || waitingForApproval {
		_, err = m.ConfirmJoiningGroup(context.Background(), chat.ID)
		if err != nil {
			return err
		}
	}

	if message.Message != nil {
		messageState.CurrentMessageState.Message = *message.Message
		return m.HandleChatMessage(messageState)
	} else if message.EmojiReaction != nil {
		return m.HandleEmojiReaction(messageState, *message.EmojiReaction)
	}

	return nil
}

func (m *Messenger) checkIfCreatorIsOurContact(group *v1protocol.Group) bool {
	creator, err := group.Creator()
	if err == nil {
		contact, _ := m.allContacts.Load(creator)
		return contact != nil && contact.mutual()
	}
	m.logger.Warn("failed to get creator from group", zap.String("group name", group.Name()), zap.String("group chat id", group.ChatID()), zap.Error(err))
	return false
}

func (m *Messenger) createMessageNotification(chat *Chat, messageState *ReceivedMessageState, message *common.Message) {

	var notificationType ActivityCenterType
	if chat.OneToOne() {
		notificationType = ActivityCenterNotificationTypeNewOneToOne
	} else {
		notificationType = ActivityCenterNotificationTypeNewPrivateGroupChat
	}
	notification := &ActivityCenterNotification{
		ID:          types.FromHex(chat.ID),
		Name:        chat.Name,
		LastMessage: message,
		Type:        notificationType,
		Author:      messageState.CurrentMessageState.Contact.ID,
		Timestamp:   messageState.CurrentMessageState.WhisperTimestamp,
		ChatID:      chat.ID,
		CommunityID: chat.CommunityID,
		UpdatedAt:   m.getCurrentTimeInMillis(),
	}

	err := m.addActivityCenterNotification(messageState.Response, notification)
	if err != nil {
		m.logger.Warn("failed to create activity center notification", zap.Error(err))
	}
}

func (m *Messenger) PendingNotificationContactRequest(contactID string) (*ActivityCenterNotification, error) {
	return m.persistence.ActiveContactRequestNotification(contactID)
}

func (m *Messenger) createContactRequestForContactUpdate(contact *Contact, messageState *ReceivedMessageState) (*common.Message, error) {
	contactRequest, err := m.generateContactRequest(
		messageState.CurrentMessageState.Message.Clock,
		messageState.CurrentMessageState.WhisperTimestamp,
		contact,
		defaultContactRequestText(),
		false,
	)
	if err != nil {
		return nil, err
	}

	contactRequest.ID = defaultContactRequestID(contact.ID)

	// save this message
	messageState.Response.AddMessage(contactRequest)
	err = m.persistence.SaveMessages([]*common.Message{contactRequest})

	if err != nil {
		return nil, err
	}

	return contactRequest, nil
}

func (m *Messenger) createIncomingContactRequestNotification(contact *Contact, messageState *ReceivedMessageState, contactRequest *common.Message, createNewNotification bool) error {
	if contactRequest.ContactRequestState == common.ContactRequestStateAccepted {
		// Pull one from the db if there
		notification, err := m.persistence.GetActivityCenterNotificationByID(types.FromHex(contactRequest.ID))
		if err != nil {
			return err
		}

		if notification != nil {
			notification.Name = contact.PrimaryName()
			notification.Message = contactRequest
			notification.Read = true
			notification.Accepted = true
			notification.Dismissed = false
			notification.UpdatedAt = m.getCurrentTimeInMillis()
			_, err = m.persistence.SaveActivityCenterNotification(notification, true)
			if err != nil {
				return err
			}
			messageState.Response.AddMessage(contactRequest)
			messageState.Response.AddActivityCenterNotification(notification)

			err = m.syncActivityCenterNotifications([]*ActivityCenterNotification{notification})
			if err != nil {
				m.logger.Error("createIncomingContactRequestNotification, failed to sync activity center notifications", zap.Error(err))
				return err
			}
		}
		return nil
	}

	if !createNewNotification {
		return nil
	}

	notification := &ActivityCenterNotification{
		ID:        types.FromHex(contactRequest.ID),
		Name:      contact.PrimaryName(),
		Message:   contactRequest,
		Type:      ActivityCenterNotificationTypeContactRequest,
		Author:    contactRequest.From,
		Timestamp: contactRequest.WhisperTimestamp,
		ChatID:    contact.ID,
		Read:      contactRequest.ContactRequestState == common.ContactRequestStateAccepted || contactRequest.ContactRequestState == common.ContactRequestStateDismissed,
		Accepted:  contactRequest.ContactRequestState == common.ContactRequestStateAccepted,
		Dismissed: contactRequest.ContactRequestState == common.ContactRequestStateDismissed,
		UpdatedAt: m.getCurrentTimeInMillis(),
	}

	return m.addActivityCenterNotification(messageState.Response, notification)
}

func (m *Messenger) handleCommandMessage(state *ReceivedMessageState, message *common.Message) error {
	message.ID = state.CurrentMessageState.MessageID
	message.From = state.CurrentMessageState.Contact.ID
	message.Alias = state.CurrentMessageState.Contact.Alias
	message.SigPubKey = state.CurrentMessageState.PublicKey
	message.Identicon = state.CurrentMessageState.Contact.Identicon
	message.WhisperTimestamp = state.CurrentMessageState.WhisperTimestamp

	if err := message.PrepareContent(common.PubkeyToHex(&m.identity.PublicKey)); err != nil {
		return fmt.Errorf("failed to prepare content: %v", err)
	}
	chat, err := m.matchChatEntity(message)
	if err != nil {
		return err
	}

	allowed, err := m.isMessageAllowedFrom(state.CurrentMessageState.Contact.ID, chat)
	if err != nil {
		return err
	}

	if !allowed {
		return ErrMessageNotAllowed
	}

	// If deleted-at is greater, ignore message
	if chat.DeletedAtClockValue >= message.Clock {
		return nil
	}

	// Set the LocalChatID for the message
	message.LocalChatID = chat.ID

	if c, ok := state.AllChats.Load(chat.ID); ok {
		chat = c
	}

	// Set the LocalChatID for the message
	message.LocalChatID = chat.ID

	// Increase unviewed count
	if !common.IsPubKeyEqual(message.SigPubKey, &m.identity.PublicKey) {
		m.updateUnviewedCounts(chat, message.Mentioned || message.Replied)
		message.OutgoingStatus = ""
	} else {
		// Our own message, mark as sent
		message.OutgoingStatus = common.OutgoingStatusSent
	}

	err = chat.UpdateFromMessage(message, state.Timesource)
	if err != nil {
		return err
	}

	if !chat.Active {
		m.createMessageNotification(chat, state, chat.LastMessage)
	}

	// Add to response
	state.Response.AddChat(chat)
	if message != nil {
		message.New = true
		state.Response.AddMessage(message)
	}

	// Set in the modified maps chat
	state.AllChats.Store(chat.ID, chat)

	return nil
}

func (m *Messenger) syncContactRequestForInstallationContact(contact *Contact, state *ReceivedMessageState, chat *Chat, outgoing bool) error {

	if chat == nil {
		return fmt.Errorf("no chat restored during the contact synchronisation, contact.ID = %s", contact.ID)
	}

	contactRequestID, err := m.persistence.LatestPendingContactRequestIDForContact(contact.ID)
	if err != nil {
		return err
	}

	if contactRequestID != "" {
		m.logger.Warn("syncContactRequestForInstallationContact: skipping as contact request found", zap.String("contactRequestID", contactRequestID))
		return nil
	}

	clock, timestamp := chat.NextClockAndTimestamp(m.transport)
	contactRequest, err := m.generateContactRequest(clock, timestamp, contact, defaultContactRequestText(), outgoing)
	if err != nil {
		return err
	}

	contactRequest.ID = defaultContactRequestID(contact.ID)

	state.Response.AddMessage(contactRequest)
	err = m.persistence.SaveMessages([]*common.Message{contactRequest})
	if err != nil {
		return err
	}

	if outgoing {
		notification := m.generateOutgoingContactRequestNotification(contact, contactRequest)
		err = m.addActivityCenterNotification(state.Response, notification)
		if err != nil {
			return err
		}
	} else {
		err = m.createIncomingContactRequestNotification(contact, state, contactRequest, true)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Messenger) HandleSyncInstallationContact(state *ReceivedMessageState, message protobuf.SyncInstallationContactV2) error {
	// Ignore own contact installation

	if message.Id == m.myHexIdentity() {
		m.logger.Warn("HandleSyncInstallationContact: skipping own contact")
		return nil
	}

	removedOrBlocked := message.Removed || message.Blocked
	chat, ok := state.AllChats.Load(message.Id)
	if !ok && (message.Added || message.HasAddedUs || message.Muted) && !removedOrBlocked {
		pubKey, err := common.HexToPubkey(message.Id)
		if err != nil {
			return err
		}

		chat = OneToOneFromPublicKey(pubKey, state.Timesource)
		// We don't want to show the chat to the user
		chat.Active = false
	}

	contact, ok := state.AllContacts.Load(message.Id)
	if !ok {
		if message.Removed {
			// Nothing to do in case if contact doesn't exist
			return nil
		}

		var err error
		contact, err = buildContactFromPkString(message.Id)
		if err != nil {
			return err
		}
	}

	if message.ContactRequestRemoteClock != 0 || message.ContactRequestLocalClock != 0 {
		// Some local action about contact requests were performed,
		// process them
		contact.ProcessSyncContactRequestState(
			ContactRequestState(message.ContactRequestRemoteState),
			uint64(message.ContactRequestRemoteClock),
			ContactRequestState(message.ContactRequestLocalState),
			uint64(message.ContactRequestLocalClock))
		state.ModifiedContacts.Store(contact.ID, true)
		state.AllContacts.Store(contact.ID, contact)

		err := m.syncContactRequestForInstallationContact(contact, state, chat, contact.ContactRequestLocalState == ContactRequestStateSent)
		if err != nil {
			return err
		}
	} else if message.Added || message.HasAddedUs {
		// NOTE(cammellos): this is for handling backward compatibility, old clients
		// won't propagate ContactRequestRemoteClock or ContactRequestLocalClock

		if message.Added && contact.LastUpdatedLocally < message.LastUpdatedLocally {
			contact.ContactRequestSent(message.LastUpdatedLocally)

			err := m.syncContactRequestForInstallationContact(contact, state, chat, true)
			if err != nil {
				return err
			}
		}

		if message.HasAddedUs && contact.LastUpdated < message.LastUpdated {
			contact.ContactRequestReceived(message.LastUpdated)

			err := m.syncContactRequestForInstallationContact(contact, state, chat, false)
			if err != nil {
				return err
			}
		}

		if message.Removed && contact.LastUpdatedLocally < message.LastUpdatedLocally {
			err := m.removeContact(context.Background(), state.Response, contact.ID, false)
			if err != nil {
				return err
			}
		}
	}

	// Sync last updated field
	// We don't set `LastUpdated`, since that would cause some issues
	// as `LastUpdated` tracks both display name & picture.
	// The case where it would break is as follow:
	// 1) User A pairs A1 with device A2.
	// 2) User B publishes display name and picture with LastUpdated = 3.
	// 3) Device A1 receives message from step 2.
	// 4) Device A1 syncs with A2 (which has not received message from step 3).
	// 5) Device A2 saves Display name and sets LastUpdated = 3,
	//    note that picture has not been set as it's not synced.
	// 6) Device A2 receives the message from 2. because LastUpdated is 3
	//    it will be discarded, A2 will not have B's picture.
	// The correct solution is to either sync profile image (expensive)
	// or split the clock for image/display name, so they can be synced
	// independently.
	if contact.LastUpdated < message.LastUpdated {
		if message.DisplayName != "" {
			contact.DisplayName = message.DisplayName
		}
		state.ModifiedContacts.Store(contact.ID, true)
		state.AllContacts.Store(contact.ID, contact)
	}

	if contact.LastUpdatedLocally < message.LastUpdatedLocally {
		// NOTE(cammellos): probably is cleaner to pass a flag
		// to method to tell them not to sync, or factor out in different
		// methods
		contact.IsSyncing = true
		defer func() {
			contact.IsSyncing = false
		}()

		if message.EnsName != "" && contact.EnsName != message.EnsName {
			contact.EnsName = message.EnsName
			publicKey, err := contact.PublicKey()
			if err != nil {
				return err
			}

			err = m.ENSVerified(common.PubkeyToHex(publicKey), message.EnsName)
			if err != nil {
				contact.ENSVerified = false
			}
			contact.ENSVerified = true
		}
		contact.LastUpdatedLocally = message.LastUpdatedLocally
		contact.LocalNickname = message.LocalNickname
		contact.TrustStatus = verification.TrustStatus(message.TrustStatus)
		contact.VerificationStatus = VerificationStatus(message.VerificationStatus)

		_, err := m.verificationDatabase.UpsertTrustStatus(contact.ID, contact.TrustStatus, message.LastUpdatedLocally)
		if err != nil {
			return err
		}

		if message.Blocked != contact.Blocked {
			if message.Blocked {
				state.AllContacts.Store(contact.ID, contact)
				response, err := m.BlockContact(contact.ID)
				if err != nil {
					return err
				}
				err = state.Response.Merge(response)
				if err != nil {
					return err
				}
			} else {
				contact.Unblock(message.LastUpdatedLocally)
			}
		}
		if chat != nil && message.Muted != chat.Muted {
			if message.Muted {
				_, err := m.muteChat(chat, contact, time.Time{})
				if err != nil {
					return err
				}
			} else {
				err := m.unmuteChat(chat, contact)
				if err != nil {
					return err
				}
			}

			state.Response.AddChat(chat)
		}

		state.ModifiedContacts.Store(contact.ID, true)
		state.AllContacts.Store(contact.ID, contact)
	}

	if chat != nil {
		state.AllChats.Store(chat.ID, chat)
	}

	return nil
}

func (m *Messenger) HandleSyncProfilePictures(state *ReceivedMessageState, message protobuf.SyncProfilePictures) error {
	dbImages, err := m.multiAccounts.GetIdentityImages(message.KeyUid)
	if err != nil {
		return err
	}
	dbImageMap := make(map[string]*images.IdentityImage)
	for _, img := range dbImages {
		dbImageMap[img.Name] = img
	}
	idImages := make([]images.IdentityImage, len(message.Pictures))
	i := 0
	for _, message := range message.Pictures {
		dbImg := dbImageMap[message.Name]
		if dbImg != nil && message.Clock <= dbImg.Clock {
			continue
		}
		image := images.IdentityImage{
			Name:         message.Name,
			Payload:      message.Payload,
			Width:        int(message.Width),
			Height:       int(message.Height),
			FileSize:     int(message.FileSize),
			ResizeTarget: int(message.ResizeTarget),
			Clock:        message.Clock,
		}
		idImages[i] = image
		i++
	}

	if i == 0 {
		return nil
	}

	err = m.multiAccounts.StoreIdentityImages(message.KeyUid, idImages[:i], false)
	if err == nil {
		state.Response.IdentityImages = idImages[:i]
	}
	return err
}

func (m *Messenger) HandleSyncInstallationPublicChat(state *ReceivedMessageState, message protobuf.SyncInstallationPublicChat) *Chat {
	chatID := message.Id
	existingChat, ok := state.AllChats.Load(chatID)
	if ok && (existingChat.Active || uint32(message.GetClock()/1000) < existingChat.SyncedTo) {
		return nil
	}

	chat := existingChat
	if !ok {
		chat = CreatePublicChat(chatID, state.Timesource)
		chat.Joined = int64(message.Clock)
	} else {
		existingChat.Joined = int64(message.Clock)
	}

	state.AllChats.Store(chat.ID, chat)

	state.Response.AddChat(chat)
	return chat
}

func (m *Messenger) HandleSyncChatRemoved(state *ReceivedMessageState, message protobuf.SyncChatRemoved) error {
	chat, ok := m.allChats.Load(message.Id)
	if !ok {
		return ErrChatNotFound
	}

	if chat.Joined > int64(message.Clock) {
		return nil
	}

	if chat.DeletedAtClockValue > message.Clock {
		return nil
	}

	if chat.PrivateGroupChat() {
		_, err := m.leaveGroupChat(context.Background(), state.Response, message.Id, true, false)
		if err != nil {
			return err
		}
	}

	response, err := m.deactivateChat(message.Id, message.Clock, false, true)
	if err != nil {
		return err
	}

	return state.Response.Merge(response)
}

func (m *Messenger) HandleSyncChatMessagesRead(state *ReceivedMessageState, message protobuf.SyncChatMessagesRead) error {
	chat, ok := m.allChats.Load(message.Id)
	if !ok {
		return ErrChatNotFound
	}

	if chat.ReadMessagesAtClockValue > message.Clock {
		return nil
	}

	err := m.markAllRead(message.Id, message.Clock, false)
	if err != nil {
		return err
	}

	state.Response.AddChat(chat)
	return nil
}

func (m *Messenger) handlePinMessage(pinner *Contact, whisperTimestamp uint64, response *MessengerResponse, message protobuf.PinMessage) error {
	logger := m.logger.With(zap.String("site", "HandlePinMessage"))

	logger.Info("Handling pin message")

	publicKey, err := pinner.PublicKey()
	if err != nil {
		return err
	}

	pinMessage := &common.PinMessage{
		PinMessage: message,
		// MessageID:        message.MessageId,
		WhisperTimestamp: whisperTimestamp,
		From:             pinner.ID,
		SigPubKey:        publicKey,
		Identicon:        pinner.Identicon,
		Alias:            pinner.Alias,
	}

	chat, err := m.matchChatEntity(pinMessage)
	if err != nil {
		return err // matchChatEntity returns a descriptive error message
	}

	pinMessage.ID, err = generatePinMessageID(&m.identity.PublicKey, pinMessage, chat)
	if err != nil {
		return err
	}

	// If deleted-at is greater, ignore message
	if chat.DeletedAtClockValue >= pinMessage.Clock {
		return nil
	}

	// Set the LocalChatID for the message
	pinMessage.LocalChatID = chat.ID

	if c, ok := m.allChats.Load(chat.ID); ok {
		chat = c
	}

	// Set the LocalChatID for the message
	pinMessage.LocalChatID = chat.ID

	inserted, err := m.persistence.SavePinMessage(pinMessage)
	if err != nil {
		return err
	}

	// Nothing to do, returning
	if !inserted {
		m.logger.Info("pin message already processed")
		return nil
	}

	if message.Pinned {
		id, err := generatePinMessageNotificationID(&m.identity.PublicKey, pinMessage, chat)
		if err != nil {
			return err
		}
		message := &common.Message{
			ChatMessage: protobuf.ChatMessage{
				Clock:       message.Clock,
				Timestamp:   whisperTimestamp,
				ChatId:      chat.ID,
				MessageType: message.MessageType,
				ResponseTo:  message.MessageId,
				ContentType: protobuf.ChatMessage_SYSTEM_MESSAGE_PINNED_MESSAGE,
			},
			WhisperTimestamp: whisperTimestamp,
			ID:               id,
			LocalChatID:      chat.ID,
			From:             pinner.ID,
		}
		response.AddMessage(message)
		chat.UnviewedMessagesCount++
	}

	if chat.LastClockValue < message.Clock {
		chat.LastClockValue = message.Clock
	}

	response.AddPinMessage(pinMessage)

	// Set in the modified maps chat
	response.AddChat(chat)
	m.allChats.Store(chat.ID, chat)
	return nil
}

func (m *Messenger) HandlePinMessage(state *ReceivedMessageState, message protobuf.PinMessage) error {
	return m.handlePinMessage(state.CurrentMessageState.Contact, state.CurrentMessageState.WhisperTimestamp, state.Response, message)
}

func (m *Messenger) handleAcceptContactRequest(
	response *MessengerResponse,
	contact *Contact,
	originalRequest *common.Message,
	clock uint64) (ContactRequestProcessingResponse, error) {

	m.logger.Debug("received contact request", zap.Uint64("clock-sent", clock), zap.Uint64("current-clock", contact.ContactRequestRemoteClock), zap.Uint64("current-state", uint64(contact.ContactRequestRemoteState)))
	if contact.ContactRequestRemoteClock > clock {
		m.logger.Debug("not handling accept since clock lower")
		return ContactRequestProcessingResponse{}, nil
	}

	// The contact request accepted wasn't found, a reason for this might
	// be that we sent a legacy contact request/contact-update, or another
	// device has sent it, and we haven't synchronized it
	if originalRequest == nil {
		return contact.ContactRequestAccepted(clock), nil
	}

	if originalRequest.LocalChatID != contact.ID {
		return ContactRequestProcessingResponse{}, errors.New("can't accept contact request not sent to user")
	}

	contact.ContactRequestAccepted(clock)

	originalRequest.ContactRequestState = common.ContactRequestStateAccepted

	err := m.persistence.SetContactRequestState(originalRequest.ID, originalRequest.ContactRequestState)
	if err != nil {
		return ContactRequestProcessingResponse{}, err
	}

	response.AddMessage(originalRequest)
	return ContactRequestProcessingResponse{}, nil
}

func (m *Messenger) handleAcceptContactRequestMessage(state *ReceivedMessageState, clock uint64, contactRequestID string, isOutgoing bool) error {
	request, err := m.persistence.MessageByID(contactRequestID)
	if err != nil && err != common.ErrRecordNotFound {
		return err
	}

	contact := state.CurrentMessageState.Contact

	processingResponse, err := m.handleAcceptContactRequest(state.Response, contact, request, clock)
	if err != nil {
		return err
	}

	// If the state has changed from non-mutual contact, to mutual contact
	// we want to notify the user
	if contact.mutual() {
		// We set the chat as active, this is currently the expected behavior
		// for mobile, it might change as we implement further the activity
		// center
		chat, _, err := m.getOneToOneAndNextClock(contact)
		if err != nil {
			return err
		}

		if chat.LastClockValue < clock {
			chat.LastClockValue = clock
		}

		// NOTE(cammellos): This will re-enable the chat if it was deleted, and only
		// after we became contact, currently seems safe, but that needs
		// discussing with UX.
		if chat.DeletedAtClockValue < clock {
			chat.Active = true
		}

		// Add mutual state update message for incoming contact request
		clock, timestamp := chat.NextClockAndTimestamp(m.transport)

		updateMessage, err := m.prepareMutualStateUpdateMessage(contact.ID, MutualStateUpdateTypeAdded, clock, timestamp, false)
		if err != nil {
			return err
		}

		m.prepareMessage(updateMessage, m.httpServer)
		err = m.persistence.SaveMessages([]*common.Message{updateMessage})
		if err != nil {
			return err
		}
		state.Response.AddMessage(updateMessage)

		state.Response.AddChat(chat)
		state.AllChats.Store(chat.ID, chat)
	}

	if request != nil {
		if isOutgoing {
			notification := m.generateOutgoingContactRequestNotification(contact, request)
			err = m.addActivityCenterNotification(state.Response, notification)
			if err != nil {
				return err
			}
		} else {
			err = m.createIncomingContactRequestNotification(contact, state, request, processingResponse.newContactRequestReceived)
			if err != nil {
				return err
			}
		}
		if err != nil {
			m.logger.Warn("could not create contact request notification", zap.Error(err))
		}
	}

	state.ModifiedContacts.Store(contact.ID, true)
	state.AllContacts.Store(contact.ID, contact)
	return nil
}

func (m *Messenger) HandleAcceptContactRequest(state *ReceivedMessageState, message protobuf.AcceptContactRequest, senderID string) error {
	err := m.handleAcceptContactRequestMessage(state, message.Clock, message.Id, false)
	if err != nil {
		m.logger.Warn("could not accept contact request", zap.Error(err))
	}

	return nil
}

func (m *Messenger) handleRetractContactRequest(response *MessengerResponse, contact *Contact, message protobuf.RetractContactRequest) error {
	if contact.ID == m.myHexIdentity() {
		m.logger.Debug("retraction coming from us, ignoring")
		return nil
	}

	m.logger.Debug("handling retracted contact request", zap.Uint64("clock", message.Clock))
	r := contact.ContactRequestRetracted(message.Clock, false)
	if !r.processed {
		m.logger.Debug("not handling retract since clock lower")
		return nil
	}

	// System message for mutual state update
	chat, clock, err := m.getOneToOneAndNextClock(contact)
	if err != nil {
		return err
	}
	timestamp := m.getTimesource().GetCurrentTime()
	updateMessage, err := m.prepareMutualStateUpdateMessage(contact.ID, MutualStateUpdateTypeRemoved, clock, timestamp, false)
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

	notification := &ActivityCenterNotification{
		ID:        types.FromHex(uuid.New().String()),
		Type:      ActivityCenterNotificationTypeContactRemoved,
		Name:      contact.PrimaryName(),
		Author:    contact.ID,
		Timestamp: m.getTimesource().GetCurrentTime(),
		ChatID:    contact.ID,
		Read:      false,
		UpdatedAt: m.getCurrentTimeInMillis(),
	}

	err = m.addActivityCenterNotification(response, notification)
	if err != nil {
		m.logger.Warn("failed to create activity center notification", zap.Error(err))
		return err
	}

	m.allContacts.Store(contact.ID, contact)

	return nil
}

func (m *Messenger) HandleRetractContactRequest(state *ReceivedMessageState, message protobuf.RetractContactRequest) error {
	contact := state.CurrentMessageState.Contact
	err := m.handleRetractContactRequest(state.Response, contact, message)
	if err != nil {
		return err
	}
	if contact.ID != m.myHexIdentity() {
		state.ModifiedContacts.Store(contact.ID, true)
	}

	return nil
}

func (m *Messenger) HandleContactUpdate(state *ReceivedMessageState, message protobuf.ContactUpdate) error {
	logger := m.logger.With(zap.String("site", "HandleContactUpdate"))
	contact := state.CurrentMessageState.Contact
	chat, ok := state.AllChats.Load(contact.ID)

	allowed, err := m.isMessageAllowedFrom(state.CurrentMessageState.Contact.ID, chat)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrMessageNotAllowed
	}

	if err = ValidateDisplayName(&message.DisplayName); err != nil {
		return err
	}

	if !ok {
		chat = OneToOneFromPublicKey(state.CurrentMessageState.PublicKey, state.Timesource)
		// We don't want to show the chat to the user
		chat.Active = false
	}

	logger.Debug("Handling contact update")

	if message.ContactRequestPropagatedState != nil {
		logger.Debug("handling contact request propagated state", zap.Any("state before update", contact.ContactRequestPropagatedState()))
		result := contact.ContactRequestPropagatedStateReceived(message.ContactRequestPropagatedState)
		if result.sendBackState {
			logger.Debug("sending back state")
			// This is a bit dangerous, since it might trigger a ping-pong of contact updates
			// also it should backoff/debounce
			_, err = m.sendContactUpdate(context.Background(), contact.ID, "", "", "", m.dispatchMessage)
			if err != nil {
				return err
			}

		}
		if result.newContactRequestReceived {
			contactRequest, err := m.createContactRequestForContactUpdate(contact, state)
			if err != nil {
				return err
			}

			err = m.createIncomingContactRequestNotification(contact, state, contactRequest, true)
			if err != nil {
				return err
			}
		}

		logger.Debug("handled propagated state", zap.Any("state after update", contact.ContactRequestPropagatedState()))
		state.ModifiedContacts.Store(contact.ID, true)
		state.AllContacts.Store(contact.ID, contact)
	}

	if contact.LastUpdated < message.Clock {
		if contact.EnsName != message.EnsName {
			contact.EnsName = message.EnsName
			contact.ENSVerified = false
		}

		if len(message.DisplayName) != 0 {
			contact.DisplayName = message.DisplayName
		}

		r := contact.ContactRequestReceived(message.ContactRequestClock)
		if r.newContactRequestReceived {
			err = m.createIncomingContactRequestNotification(contact, state, nil, true)
			if err != nil {
				return err
			}
		}
		contact.LastUpdated = message.Clock
		state.ModifiedContacts.Store(contact.ID, true)
		state.AllContacts.Store(contact.ID, contact)
	}

	if chat.LastClockValue < message.Clock {
		chat.LastClockValue = message.Clock
	}

	if contact.mutual() && chat.DeletedAtClockValue < message.Clock {
		chat.Active = true
	}

	state.Response.AddChat(chat)
	// TODO(samyoul) remove storing of an updated reference pointer?
	state.AllChats.Store(chat.ID, chat)

	return nil
}

func (m *Messenger) HandlePairInstallation(state *ReceivedMessageState, message protobuf.PairInstallation) error {
	logger := m.logger.With(zap.String("site", "HandlePairInstallation"))
	if err := ValidateReceivedPairInstallation(&message, state.CurrentMessageState.WhisperTimestamp); err != nil {
		logger.Warn("failed to validate message", zap.Error(err))
		return err
	}

	installation, ok := state.AllInstallations.Load(message.InstallationId)
	if !ok {
		return errors.New("installation not found")
	}

	metadata := &multidevice.InstallationMetadata{
		Name:       message.Name,
		DeviceType: message.DeviceType,
	}

	installation.InstallationMetadata = metadata
	// TODO(samyoul) remove storing of an updated reference pointer?
	state.AllInstallations.Store(message.InstallationId, installation)
	state.ModifiedInstallations.Store(message.InstallationId, true)

	return nil
}

// HandleCommunityInvitation handles an community invitation
func (m *Messenger) HandleCommunityInvitation(state *ReceivedMessageState, signer *ecdsa.PublicKey, invitation protobuf.CommunityInvitation, rawPayload []byte) error {
	if invitation.PublicKey == nil {
		return errors.New("invalid pubkey")
	}
	pk, err := crypto.DecompressPubkey(invitation.PublicKey)
	if err != nil {
		return err
	}

	if !common.IsPubKeyEqual(pk, &m.identity.PublicKey) {
		return errors.New("invitation not for us")
	}

	communityResponse, err := m.communitiesManager.HandleCommunityInvitation(signer, &invitation, rawPayload)
	if err != nil {
		return err
	}

	community := communityResponse.Community

	state.Response.AddCommunity(community)
	state.Response.CommunityChanges = append(state.Response.CommunityChanges, communityResponse.Changes)

	return nil
}

func (m *Messenger) HandleHistoryArchiveMagnetlinkMessage(state *ReceivedMessageState, communityPubKey *ecdsa.PublicKey, magnetlink string, clock uint64) error {

	id := types.HexBytes(crypto.CompressPubkey(communityPubKey))
	settings, err := m.communitiesManager.GetCommunitySettingsByID(id)
	if err != nil {
		m.logger.Debug("Couldn't get community settings for community with id: ", zap.Any("id", id))
		return err
	}

	if m.torrentClientReady() && settings != nil && settings.HistoryArchiveSupportEnabled {
		signedByOwnedCommunity, err := m.communitiesManager.IsAdminCommunity(communityPubKey)
		if err != nil {
			return err
		}
		joinedCommunity, err := m.communitiesManager.IsJoinedCommunity(communityPubKey)
		if err != nil {
			return err
		}
		lastClock, err := m.communitiesManager.GetMagnetlinkMessageClock(id)
		if err != nil {
			return err
		}
		lastSeenMagnetlink, err := m.communitiesManager.GetLastSeenMagnetlink(id)
		if err != nil {
			return err
		}
		// We are only interested in a community archive magnet link
		// if it originates from a community that the current account is
		// part of and doesn't own the private key at the same time
		if !signedByOwnedCommunity && joinedCommunity && clock >= lastClock {
			if lastSeenMagnetlink == magnetlink {
				m.communitiesManager.LogStdout("already processed this magnetlink")
				return nil
			}

			m.communitiesManager.UnseedHistoryArchiveTorrent(id)
			currentTask := m.communitiesManager.GetHistoryArchiveDownloadTask(id.String())

			go func(currentTask *communities.HistoryArchiveDownloadTask, communityID types.HexBytes) {

				// Cancel ongoing download/import task
				if currentTask != nil && !currentTask.IsCancelled() {
					currentTask.Cancel()
					currentTask.Waiter.Wait()
				}

				// Create new task
				task := &communities.HistoryArchiveDownloadTask{
					CancelChan: make(chan struct{}),
					Waiter:     *new(sync.WaitGroup),
					Cancelled:  false,
				}

				m.communitiesManager.AddHistoryArchiveDownloadTask(communityID.String(), task)

				// this wait groups tracks the ongoing task for a particular community
				task.Waiter.Add(1)
				defer task.Waiter.Done()

				// this wait groups tracks all ongoing tasks across communities
				m.downloadHistoryArchiveTasksWaitGroup.Add(1)
				defer m.downloadHistoryArchiveTasksWaitGroup.Done()
				m.downloadAndImportHistoryArchives(communityID, magnetlink, task.CancelChan)
			}(currentTask, id)

			return m.communitiesManager.UpdateMagnetlinkMessageClock(id, clock)
		}
	}
	return nil
}

func (m *Messenger) downloadAndImportHistoryArchives(id types.HexBytes, magnetlink string, cancel chan struct{}) {
	downloadTaskInfo, err := m.communitiesManager.DownloadHistoryArchivesByMagnetlink(id, magnetlink, cancel)
	if err != nil {
		logMsg := "failed to download history archive data"
		if err == communities.ErrTorrentTimedout {
			m.communitiesManager.LogStdout("torrent has timed out, trying once more...")
			downloadTaskInfo, err = m.communitiesManager.DownloadHistoryArchivesByMagnetlink(id, magnetlink, cancel)
			if err != nil {
				m.communitiesManager.LogStdout(logMsg, zap.Error(err))
				return
			}
		} else {
			m.communitiesManager.LogStdout(logMsg, zap.Error(err))
			return
		}
	}

	if downloadTaskInfo.Cancelled {
		if downloadTaskInfo.TotalDownloadedArchivesCount > 0 {
			m.communitiesManager.LogStdout(fmt.Sprintf("downloaded %d of %d archives so far", downloadTaskInfo.TotalDownloadedArchivesCount, downloadTaskInfo.TotalArchivesCount))
		}
		return
	}

	err = m.communitiesManager.UpdateLastSeenMagnetlink(id, magnetlink)
	if err != nil {
		m.communitiesManager.LogStdout("couldn't update last seen magnetlink", zap.Error(err))
	}

	err = m.importHistoryArchives(id, cancel)
	if err != nil {
		m.communitiesManager.LogStdout("failed to import history archives", zap.Error(err))
		m.config.messengerSignalsHandler.DownloadingHistoryArchivesFinished(types.EncodeHex(id))
		return
	}

	m.config.messengerSignalsHandler.DownloadingHistoryArchivesFinished(types.EncodeHex(id))
}

func (m *Messenger) handleArchiveMessages(archiveMessages []*protobuf.WakuMessage) (*MessengerResponse, error) {

	messagesToHandle := make(map[transport.Filter][]*types.Message)

	for _, message := range archiveMessages {
		filter := m.transport.FilterByTopic(message.Topic)
		if filter != nil {
			shhMessage := &types.Message{
				Sig:          message.Sig,
				Timestamp:    uint32(message.Timestamp),
				Topic:        types.BytesToTopic(message.Topic),
				Payload:      message.Payload,
				Padding:      message.Padding,
				Hash:         message.Hash,
				ThirdPartyID: message.ThirdPartyId,
			}
			messagesToHandle[*filter] = append(messagesToHandle[*filter], shhMessage)
		}
	}

	importedMessages := make(map[transport.Filter][]*types.Message, 0)
	otherMessages := make(map[transport.Filter][]*types.Message, 0)

	for filter, messages := range messagesToHandle {
		for _, message := range messages {
			if message.ThirdPartyID != "" {
				importedMessages[filter] = append(importedMessages[filter], message)
			} else {
				otherMessages[filter] = append(otherMessages[filter], message)
			}
		}
	}

	err := m.handleImportedMessages(importedMessages)
	if err != nil {
		m.communitiesManager.LogStdout("failed to handle imported messages", zap.Error(err))
		return nil, err
	}

	response, err := m.handleRetrievedMessages(otherMessages, false)
	if err != nil {
		m.communitiesManager.LogStdout("failed to write history archive messages to database", zap.Error(err))
		return nil, err
	}

	return response, nil
}

func (m *Messenger) HandleCommunityCancelRequestToJoin(state *ReceivedMessageState, signer *ecdsa.PublicKey, cancelRequestToJoinProto protobuf.CommunityCancelRequestToJoin) error {
	if cancelRequestToJoinProto.CommunityId == nil {
		return ErrInvalidCommunityID
	}

	requestToJoin, err := m.communitiesManager.HandleCommunityCancelRequestToJoin(signer, &cancelRequestToJoinProto)
	if err != nil {
		return err
	}

	state.Response.RequestsToJoinCommunity = append(state.Response.RequestsToJoinCommunity, requestToJoin)

	// delete activity center notification
	notification, err := m.persistence.GetActivityCenterNotificationByID(requestToJoin.ID)
	if err != nil {
		return err
	}

	if notification != nil {
		notification.UpdatedAt = m.getCurrentTimeInMillis()
		err = m.persistence.DeleteActivityCenterNotificationByID(types.FromHex(requestToJoin.ID.String()), notification.UpdatedAt)
		if err != nil {
			m.logger.Error("failed to delete notification from Activity Center", zap.Error(err))
			return err
		}

		// sending signal to client to remove the activity center notification from UI
		response := &MessengerResponse{}
		notification.Deleted = true
		err = m.syncActivityCenterNotifications([]*ActivityCenterNotification{notification})
		if err != nil {
			m.logger.Error("HandleCommunityCancelRequestToJoin, failed to sync activity center notifications", zap.Error(err))
			return err
		}
		response.AddActivityCenterNotification(notification)

		signal.SendNewMessages(response)
	}

	return nil
}

// HandleCommunityRequestToJoin handles an community request to join
func (m *Messenger) HandleCommunityRequestToJoin(state *ReceivedMessageState, signer *ecdsa.PublicKey, requestToJoinProto protobuf.CommunityRequestToJoin) error {
	if requestToJoinProto.CommunityId == nil {
		return ErrInvalidCommunityID
	}

	timeNow := uint64(time.Now().Unix())

	requestTimeOutClock, err := communities.AddTimeoutToRequestToJoinClock(requestToJoinProto.Clock)
	if err != nil {
		return err
	}

	if timeNow >= requestTimeOutClock {
		return errors.New("request is expired")
	}

	requestToJoin, err := m.communitiesManager.HandleCommunityRequestToJoin(signer, &requestToJoinProto)
	if err != nil {
		return err
	}

	if requestToJoin.State == communities.RequestToJoinStateAccepted {
		accept := &requests.AcceptRequestToJoinCommunity{
			ID: requestToJoin.ID,
		}
		_, err = m.AcceptRequestToJoinCommunity(accept)
		if err != nil {
			if err == communities.ErrNoPermissionToJoin {
				requestToJoin.State = communities.RequestToJoinStateDeclined
			} else {
				return err
			}
		}
	}

	if requestToJoin.State == communities.RequestToJoinStateDeclined {
		cancel := &requests.DeclineRequestToJoinCommunity{
			ID: requestToJoin.ID,
		}
		_, err = m.DeclineRequestToJoinCommunity(cancel)
		if err != nil {
			return err
		}
		return nil
	}

	community, err := m.communitiesManager.GetByID(requestToJoinProto.CommunityId)
	if err != nil {
		return err
	}

	contactID := contactIDFromPublicKey(signer)

	contact, _ := state.AllContacts.Load(contactID)

	if len(requestToJoinProto.DisplayName) != 0 {
		contact.DisplayName = requestToJoinProto.DisplayName
		state.ModifiedContacts.Store(contact.ID, true)
		state.AllContacts.Store(contact.ID, contact)
		state.ModifiedContacts.Store(contact.ID, true)
	}

	if requestToJoin.State == communities.RequestToJoinStatePending {
		if state.Response.RequestsToJoinCommunity == nil {
			state.Response.RequestsToJoinCommunity = make([]*communities.RequestToJoin, 0)
		}
		state.Response.RequestsToJoinCommunity = append(state.Response.RequestsToJoinCommunity, requestToJoin)

		state.Response.AddNotification(NewCommunityRequestToJoinNotification(requestToJoin.ID.String(), community, contact))

		// Activity Center notification, new for pending state
		notification := &ActivityCenterNotification{
			ID:               types.FromHex(requestToJoin.ID.String()),
			Type:             ActivityCenterNotificationTypeCommunityMembershipRequest,
			Timestamp:        m.getTimesource().GetCurrentTime(),
			Author:           contact.ID,
			CommunityID:      community.IDString(),
			MembershipStatus: ActivityCenterMembershipStatusPending,
			Deleted:          false,
			UpdatedAt:        m.getCurrentTimeInMillis(),
		}

		err = m.addActivityCenterNotification(state.Response, notification)
		if err != nil {
			m.logger.Error("failed to save notification", zap.Error(err))
			return err
		}
	} else {
		// Activity Center notification, updating existing for accepted/declined
		notification, err := m.persistence.GetActivityCenterNotificationByID(requestToJoin.ID)
		if err != nil {
			return err
		}

		if notification != nil {
			if requestToJoin.State == communities.RequestToJoinStateAccepted {
				notification.MembershipStatus = ActivityCenterMembershipStatusAccepted
			} else {
				notification.MembershipStatus = ActivityCenterMembershipStatusDeclined
			}
			notification.UpdatedAt = m.getCurrentTimeInMillis()
			err = m.addActivityCenterNotification(state.Response, notification)
			if err != nil {
				m.logger.Error("failed to save notification", zap.Error(err))
				return err
			}
		}
	}

	return nil
}

// HandleCommunityEditSharedAddresses handles an edit a user has made to their shared addresses
func (m *Messenger) HandleCommunityEditSharedAddresses(state *ReceivedMessageState, signer *ecdsa.PublicKey, editRevealedAddressesProto protobuf.CommunityEditRevealedAccounts) error {
	if editRevealedAddressesProto.CommunityId == nil {
		return ErrInvalidCommunityID
	}

	err := m.communitiesManager.HandleCommunityEditSharedAddresses(signer, &editRevealedAddressesProto)
	if err != nil {
		return err
	}

	community, err := m.communitiesManager.GetByIDString(string(editRevealedAddressesProto.GetCommunityId()))
	if err != nil {
		return err
	}

	state.Response.AddCommunity(community)
	return nil
}

func (m *Messenger) HandleCommunityRequestToJoinResponse(state *ReceivedMessageState, signer *ecdsa.PublicKey, requestToJoinResponseProto protobuf.CommunityRequestToJoinResponse) error {
	if requestToJoinResponseProto.CommunityId == nil {
		return ErrInvalidCommunityID
	}

	updatedRequest, err := m.communitiesManager.HandleCommunityRequestToJoinResponse(signer, &requestToJoinResponseProto)
	if err != nil {
		return err
	}

	if updatedRequest != nil {
		state.Response.RequestsToJoinCommunity = append(state.Response.RequestsToJoinCommunity, updatedRequest)
	}

	if requestToJoinResponseProto.Accepted {
		response, err := m.JoinCommunity(context.Background(), requestToJoinResponseProto.CommunityId, false)
		if err != nil {
			return err
		}
		if len(response.Communities()) > 0 {
			communitySettings := response.CommunitiesSettings()[0]
			community := response.Communities()[0]
			state.Response.AddCommunity(community)
			state.Response.AddCommunitySettings(communitySettings)

			magnetlink := requestToJoinResponseProto.MagnetUri
			if m.torrentClientReady() && communitySettings != nil && communitySettings.HistoryArchiveSupportEnabled && magnetlink != "" {

				currentTask := m.communitiesManager.GetHistoryArchiveDownloadTask(community.IDString())
				go func(currentTask *communities.HistoryArchiveDownloadTask) {

					// Cancel ongoing download/import task
					if currentTask != nil && !currentTask.IsCancelled() {
						currentTask.Cancel()
						currentTask.Waiter.Wait()
					}

					task := &communities.HistoryArchiveDownloadTask{
						CancelChan: make(chan struct{}),
						Waiter:     *new(sync.WaitGroup),
						Cancelled:  false,
					}
					m.communitiesManager.AddHistoryArchiveDownloadTask(community.IDString(), task)

					task.Waiter.Add(1)
					defer task.Waiter.Done()

					m.downloadHistoryArchiveTasksWaitGroup.Add(1)
					defer m.downloadHistoryArchiveTasksWaitGroup.Done()

					m.downloadAndImportHistoryArchives(community.ID(), magnetlink, task.CancelChan)
				}(currentTask)

				clock := requestToJoinResponseProto.Community.ArchiveMagnetlinkClock
				return m.communitiesManager.UpdateMagnetlinkMessageClock(community.ID(), clock)
			}
		}
	}

	// Activity Center notification
	requestID := communities.CalculateRequestID(common.PubkeyToHex(&m.identity.PublicKey), requestToJoinResponseProto.CommunityId)
	notification, err := m.persistence.GetActivityCenterNotificationByID(requestID)
	if err != nil {
		return err
	}

	if notification != nil {
		if requestToJoinResponseProto.Accepted {
			notification.MembershipStatus = ActivityCenterMembershipStatusAccepted
			notification.Read = false
			notification.Deleted = false
		} else {
			notification.MembershipStatus = ActivityCenterMembershipStatusDeclined
		}
		notification.UpdatedAt = m.getCurrentTimeInMillis()
		err = m.addActivityCenterNotification(state.Response, notification)
		if err != nil {
			m.logger.Warn("failed to update notification", zap.Error(err))
			return err
		}
	}

	return nil
}

func (m *Messenger) HandleCommunityRequestToLeave(state *ReceivedMessageState, signer *ecdsa.PublicKey, requestToLeaveProto protobuf.CommunityRequestToLeave) error {
	if requestToLeaveProto.CommunityId == nil {
		return ErrInvalidCommunityID
	}

	err := m.communitiesManager.HandleCommunityRequestToLeave(signer, &requestToLeaveProto)
	if err != nil {
		return err
	}

	response, err := m.RemoveUserFromCommunity(requestToLeaveProto.CommunityId, common.PubkeyToHex(signer))
	if err != nil {
		return err
	}

	if len(response.Communities()) > 0 {
		state.Response.AddCommunity(response.Communities()[0])
	}

	return nil
}

// handleWrappedCommunityDescriptionMessage handles a wrapped community description
func (m *Messenger) handleWrappedCommunityDescriptionMessage(payload []byte) (*communities.CommunityResponse, error) {
	return m.communitiesManager.HandleWrappedCommunityDescriptionMessage(payload)
}

func (m *Messenger) HandleEditMessage(state *ReceivedMessageState, editMessage EditMessage) error {
	if err := ValidateEditMessage(editMessage.EditMessage); err != nil {
		return err
	}
	messageID := editMessage.MessageId

	originalMessage, err := m.getMessageFromResponseOrDatabase(state.Response, messageID)

	if err == common.ErrRecordNotFound {
		return m.persistence.SaveEdit(editMessage)
	} else if err != nil {
		return err
	}

	originalMessageMentioned := originalMessage.Mentioned

	chat, ok := m.allChats.Load(originalMessage.LocalChatID)
	if !ok {
		return errors.New("chat not found")
	}

	// Check edit is valid
	if originalMessage.From != editMessage.From {
		return errors.New("invalid edit, not the right author")
	}

	// Check that edit should be applied
	if originalMessage.EditedAt >= editMessage.Clock {
		return m.persistence.SaveEdit(editMessage)
	}

	// applyEditMessage modifies the message. Changing the variable name to make it clearer
	editedMessage := originalMessage
	// Update message and return it
	err = m.applyEditMessage(&editMessage.EditMessage, editedMessage)
	if err != nil {
		return err
	}

	needToSaveChat := false
	if chat.LastMessage != nil && chat.LastMessage.ID == editedMessage.ID {
		chat.LastMessage = editedMessage
		needToSaveChat = true
	}
	responseTo, err := m.persistence.MessageByID(editedMessage.ResponseTo)

	if err != nil && err != common.ErrRecordNotFound {
		return err
	}

	err = state.updateExistingActivityCenterNotification(m.identity.PublicKey, m, editedMessage, responseTo)
	if err != nil {
		return err
	}

	editedMessageHasMentions := editedMessage.Mentioned

	if editedMessageHasMentions && !originalMessageMentioned && !editedMessage.Seen {
		// Increase unviewed count when the edited message has a mention and didn't have one before
		chat.UnviewedMentionsCount++
		needToSaveChat = true
	} else if !editedMessageHasMentions && originalMessageMentioned && !editedMessage.Seen {
		// Opposite of above, the message had a mention, but no longer does, so we reduce the count
		chat.UnviewedMentionsCount--
		needToSaveChat = true
	}
	if needToSaveChat {
		err := m.saveChat(chat)
		if err != nil {
			return err
		}
	}

	state.Response.AddMessage(editedMessage)

	// pull updated messages
	updatedMessages, err := m.persistence.MessagesByResponseTo(messageID)
	if err != nil {
		return err
	}
	state.Response.AddMessages(updatedMessages)

	state.Response.AddChat(chat)

	return nil
}

func (m *Messenger) HandleDeleteMessage(state *ReceivedMessageState, deleteMessage DeleteMessage) error {
	if err := ValidateDeleteMessage(deleteMessage.DeleteMessage); err != nil {
		return err
	}

	messageID := deleteMessage.MessageId
	// Check if it's already in the response
	originalMessage := state.Response.GetMessage(messageID)
	// otherwise pull from database
	if originalMessage == nil {
		var err error
		originalMessage, err = m.persistence.MessageByID(messageID)

		if err != nil && err != common.ErrRecordNotFound {
			return err
		}
	}

	if originalMessage == nil {
		return m.persistence.SaveDelete(deleteMessage)
	}

	chat, ok := m.allChats.Load(originalMessage.LocalChatID)
	if !ok {
		return errors.New("chat not found")
	}

	var canDeleteMessageForEveryone = false
	if originalMessage.From != deleteMessage.From {
		fromPublicKey, err := common.HexToPubkey(deleteMessage.From)
		if err != nil {
			return err
		}
		if chat.ChatType == ChatTypeCommunityChat {
			canDeleteMessageForEveryone = m.CanDeleteMessageForEveryoneInCommunity(chat.CommunityID, fromPublicKey)
			if !canDeleteMessageForEveryone {
				return ErrInvalidDeletePermission
			}
		} else if chat.ChatType == ChatTypePrivateGroupChat {
			canDeleteMessageForEveryone = m.CanDeleteMessageForEveryoneInPrivateGroupChat(chat, fromPublicKey)
			if !canDeleteMessageForEveryone {
				return ErrInvalidDeletePermission
			}
		}

		// Check edit is valid
		if !canDeleteMessageForEveryone {
			return errors.New("invalid delete, not the right author")
		}
	}

	messagesToDelete, err := m.getConnectedMessages(originalMessage, originalMessage.LocalChatID)
	if err != nil {
		return err
	}

	unreadCountDecreased := false
	for _, messageToDelete := range messagesToDelete {
		messageToDelete.Deleted = true
		messageToDelete.DeletedBy = deleteMessage.DeleteMessage.DeletedBy
		err := m.persistence.SaveMessages([]*common.Message{messageToDelete})
		if err != nil {
			return err
		}

		m.logger.Debug("deleting activity center notification for message", zap.String("chatID", chat.ID), zap.String("messageID", messageToDelete.ID))
		notifications, err := m.persistence.DeleteActivityCenterNotificationForMessage(chat.ID, messageToDelete.ID, m.getCurrentTimeInMillis())

		if err != nil {
			m.logger.Warn("failed to delete notifications for deleted message", zap.Error(err))
			return err
		}

		// Reduce chat mention count and unread count if unread
		if !messageToDelete.Seen && !unreadCountDecreased {
			unreadCountDecreased = true
			if chat.UnviewedMessagesCount > 0 {
				chat.UnviewedMessagesCount--
			}
			if chat.UnviewedMentionsCount > 0 && (messageToDelete.Mentioned || messageToDelete.Replied) {
				chat.UnviewedMentionsCount--
			}
			err := m.saveChat(chat)
			if err != nil {
				return err
			}
		}

		err = m.syncActivityCenterNotifications(notifications)
		if err != nil {
			m.logger.Error("HandleDeleteMessage, failed to sync activity center notifications", zap.Error(err))
			return err
		}

		state.Response.AddRemovedMessage(&RemovedMessage{MessageID: messageToDelete.ID, ChatID: chat.ID, DeletedBy: deleteMessage.DeleteMessage.DeletedBy})
		state.Response.AddNotification(DeletedMessageNotification(messageToDelete.ID, chat))
		state.Response.AddActivityCenterNotification(&ActivityCenterNotification{
			ID:      types.FromHex(messageToDelete.ID),
			Deleted: true,
		})

		if chat.LastMessage != nil && chat.LastMessage.ID == messageToDelete.ID {
			chat.LastMessage = messageToDelete
			err = m.saveChat(chat)
			if err != nil {
				return nil
			}
		}

		messages, err := m.persistence.LatestMessageByChatID(chat.ID)
		if err != nil {
			return err
		}
		if len(messages) > 0 {
			previousNotDeletedMessage := messages[0]
			if previousNotDeletedMessage != nil && !previousNotDeletedMessage.Seen && chat.OneToOne() && !chat.Active {
				m.createMessageNotification(chat, state, previousNotDeletedMessage)
			}
		}

		// pull updated messages
		updatedMessages, err := m.persistence.MessagesByResponseTo(messageToDelete.ID)
		if err != nil {
			return err
		}
		state.Response.AddMessages(updatedMessages)
	}

	state.Response.AddChat(chat)

	return nil
}

func (m *Messenger) getMessageFromResponseOrDatabase(response *MessengerResponse, messageID string) (*common.Message, error) {
	originalMessage := response.GetMessage(messageID)
	// otherwise pull from database
	if originalMessage != nil {
		return originalMessage, nil
	}

	return m.persistence.MessageByID(messageID)
}

func (m *Messenger) HandleDeleteForMeMessage(state *ReceivedMessageState, deleteForMeMessage protobuf.DeleteForMeMessage) error {
	if err := ValidateDeleteForMeMessage(deleteForMeMessage); err != nil {
		return err
	}

	messageID := deleteForMeMessage.MessageId
	// Check if it's already in the response
	originalMessage, err := m.getMessageFromResponseOrDatabase(state.Response, messageID)

	if err == common.ErrRecordNotFound {
		return m.persistence.SaveOrUpdateDeleteForMeMessage(&deleteForMeMessage)
	} else if err != nil {
		return err
	}

	chat, ok := m.allChats.Load(originalMessage.LocalChatID)
	if !ok {
		return errors.New("chat not found")
	}

	messagesToDelete, err := m.getConnectedMessages(originalMessage, originalMessage.LocalChatID)
	if err != nil {
		return err
	}

	for _, messageToDelete := range messagesToDelete {
		messageToDelete.DeletedForMe = true

		err := m.persistence.SaveMessages([]*common.Message{messageToDelete})
		if err != nil {
			return err
		}

		m.logger.Debug("deleting activity center notification for message", zap.String("chatID", chat.ID), zap.String("messageID", messageToDelete.ID))

		notifications, err := m.persistence.DeleteActivityCenterNotificationForMessage(chat.ID, messageToDelete.ID, m.getCurrentTimeInMillis())
		if err != nil {
			m.logger.Warn("failed to delete notifications for deleted message", zap.Error(err))
			return err
		}

		if chat.LastMessage != nil && chat.LastMessage.ID == messageToDelete.ID {
			chat.LastMessage = messageToDelete
			err = m.saveChat(chat)
			if err != nil {
				return nil
			}
		}

		err = m.syncActivityCenterNotifications(notifications)
		if err != nil {
			m.logger.Error("HandleDeleteForMeMessage, failed to sync activity center notifications", zap.Error(err))
			return err
		}

		state.Response.AddMessage(messageToDelete)
	}
	state.Response.AddChat(chat)

	return nil
}

func handleContactRequestChatMessage(receivedMessage *common.Message, contact *Contact, outgoing bool, logger *zap.Logger) (bool, error) {
	receivedMessage.ContactRequestState = common.ContactRequestStatePending

	var response ContactRequestProcessingResponse

	if outgoing {
		response = contact.ContactRequestSent(receivedMessage.Clock)
	} else {
		response = contact.ContactRequestReceived(receivedMessage.Clock)
	}
	if !response.processed {
		logger.Info("not handling contact message since clock lower")
		return false, nil

	}

	if contact.mutual() {
		receivedMessage.ContactRequestState = common.ContactRequestStateAccepted
	}

	return response.newContactRequestReceived, nil
}

func (m *Messenger) handleChatMessage(state *ReceivedMessageState, forceSeen bool) error {
	logger := m.logger.With(zap.String("site", "handleChatMessage"))
	if err := ValidateReceivedChatMessage(&state.CurrentMessageState.Message, state.CurrentMessageState.WhisperTimestamp); err != nil {
		logger.Warn("failed to validate message", zap.Error(err))
		return err
	}
	receivedMessage := &common.Message{
		ID:               state.CurrentMessageState.MessageID,
		ChatMessage:      state.CurrentMessageState.Message,
		From:             state.CurrentMessageState.Contact.ID,
		Alias:            state.CurrentMessageState.Contact.Alias,
		SigPubKey:        state.CurrentMessageState.PublicKey,
		Identicon:        state.CurrentMessageState.Contact.Identicon,
		WhisperTimestamp: state.CurrentMessageState.WhisperTimestamp,
	}

	// is the message coming from us?
	isSyncMessage := common.IsPubKeyEqual(receivedMessage.SigPubKey, &m.identity.PublicKey)

	if forceSeen || isSyncMessage {
		receivedMessage.Seen = true
	}

	err := receivedMessage.PrepareContent(m.myHexIdentity())
	if err != nil {
		return fmt.Errorf("failed to prepare message content: %v", err)
	}

	// If the message is a reply, we check if it's a reply to one of own own messages
	if receivedMessage.ResponseTo != "" {
		repliedTo, err := m.persistence.MessageByID(receivedMessage.ResponseTo)
		if err != nil && (err == sql.ErrNoRows || err == common.ErrRecordNotFound) {
			logger.Error("failed to get quoted message", zap.Error(err))
		} else if err != nil {
			return err
		} else if repliedTo.From == m.myHexIdentity() {
			receivedMessage.Replied = true
		}
	}

	chat, err := m.matchChatEntity(receivedMessage)
	if err != nil {
		return err // matchChatEntity returns a descriptive error message
	}

	if chat.ReadMessagesAtClockValue >= receivedMessage.Clock {
		receivedMessage.Seen = true
	}

	allowed, err := m.isMessageAllowedFrom(state.CurrentMessageState.Contact.ID, chat)
	if err != nil {
		return err
	}

	if !allowed {
		return ErrMessageNotAllowed
	}

	// It looks like status-mobile created profile chats as public chats
	// so for now we need to check for the presence of "@" in their chatID
	if chat.Public() && !chat.ProfileUpdates() {
		switch receivedMessage.ContentType {
		case protobuf.ChatMessage_IMAGE:
			return errors.New("images are not allowed in public chats")
		case protobuf.ChatMessage_AUDIO:
			return errors.New("audio messages are not allowed in public chats")
		}
	}

	// If profile updates check if author is the same as chat profile public key
	if chat.ProfileUpdates() && receivedMessage.From != chat.Profile {
		return nil
	}

	// If deleted-at is greater, ignore message
	if chat.DeletedAtClockValue >= receivedMessage.Clock {
		return nil
	}

	// Set the LocalChatID for the message
	receivedMessage.LocalChatID = chat.ID

	if err := m.updateChatFirstMessageTimestamp(chat, whisperToUnixTimestamp(receivedMessage.WhisperTimestamp), state.Response); err != nil {
		return err
	}

	// Our own message, mark as sent
	if isSyncMessage {
		receivedMessage.OutgoingStatus = common.OutgoingStatusSent
	} else if !receivedMessage.Seen {
		// Increase unviewed count
		skipUpdateUnviewedCountForAlbums := false
		if receivedMessage.ContentType == protobuf.ChatMessage_IMAGE {
			image := receivedMessage.GetImage()

			if image != nil && image.AlbumId != "" {
				// Skip unviewed counts increasing for other messages from album if we have it in memory
				for _, message := range state.Response.Messages() {
					if receivedMessage.ContentType == protobuf.ChatMessage_IMAGE {
						img := message.GetImage()
						if img != nil && img.AlbumId != "" && img.AlbumId == image.AlbumId {
							skipUpdateUnviewedCountForAlbums = true
							break
						}
					}
				}

				if !skipUpdateUnviewedCountForAlbums {
					messages, err := m.persistence.AlbumMessages(chat.ID, image.AlbumId)
					if err != nil {
						return err
					}

					// Skip unviewed counts increasing for other messages from album if we have it in db
					skipUpdateUnviewedCountForAlbums = len(messages) > 0
				}
			}
		}
		if !skipUpdateUnviewedCountForAlbums {
			m.updateUnviewedCounts(chat, receivedMessage.Mentioned || receivedMessage.Replied)
		}
	}

	contact := state.CurrentMessageState.Contact

	// If we receive some propagated state from someone who's not
	// our paired device, we handle it
	if receivedMessage.ContactRequestPropagatedState != nil && !isSyncMessage {
		result := contact.ContactRequestPropagatedStateReceived(receivedMessage.ContactRequestPropagatedState)
		if result.sendBackState {
			_, err = m.sendContactUpdate(context.Background(), contact.ID, "", "", "", m.dispatchMessage)
			if err != nil {
				return err
			}
		}
		if result.newContactRequestReceived {

			if contact.hasAddedUs() && !contact.mutual() {
				receivedMessage.ContactRequestState = common.ContactRequestStatePending
			}

			// Add mutual state update message for outgoing contact request
			clock := receivedMessage.Clock - 1
			updateMessage, err := m.prepareMutualStateUpdateMessage(contact.ID, MutualStateUpdateTypeSent, clock, receivedMessage.Timestamp, false)
			if err != nil {
				return err
			}

			m.prepareMessage(updateMessage, m.httpServer)
			err = m.persistence.SaveMessages([]*common.Message{updateMessage})
			if err != nil {
				return err
			}
			state.Response.AddMessage(updateMessage)

			err = m.createIncomingContactRequestNotification(contact, state, receivedMessage, true)
			if err != nil {
				return err
			}
		}
		state.ModifiedContacts.Store(contact.ID, true)
		state.AllContacts.Store(contact.ID, contact)
	}

	if receivedMessage.ContentType == protobuf.ChatMessage_CONTACT_REQUEST && chat.OneToOne() {
		chatContact := contact
		if isSyncMessage {
			chatContact, err = m.BuildContact(&requests.BuildContact{PublicKey: chat.ID})
			if err != nil {
				return err
			}
		}

		if chatContact.mutual() || chatContact.dismissed() {
			m.logger.Info("ignoring contact request message for a mutual or dismissed contact")
			return nil
		}

		sendNotification, err := handleContactRequestChatMessage(receivedMessage, chatContact, isSyncMessage, m.logger)
		if err != nil {
			m.logger.Error("failed to handle contact request message", zap.Error(err))
			return err
		}
		state.ModifiedContacts.Store(chatContact.ID, true)
		state.AllContacts.Store(chatContact.ID, chatContact)

		if sendNotification {
			err = m.createIncomingContactRequestNotification(chatContact, state, receivedMessage, true)
			if err != nil {
				return err
			}
		}
	} else if receivedMessage.ContentType == protobuf.ChatMessage_COMMUNITY {
		chat.Highlight = true
	}

	if receivedMessage.ContentType == protobuf.ChatMessage_DISCORD_MESSAGE {
		discordMessage := receivedMessage.GetDiscordMessage()
		discordMessageAuthor := discordMessage.GetAuthor()
		discordMessageAttachments := discordMessage.GetAttachments()

		state.Response.AddDiscordMessage(discordMessage)
		state.Response.AddDiscordMessageAuthor(discordMessageAuthor)

		if len(discordMessageAttachments) > 0 {
			state.Response.AddDiscordMessageAttachments(discordMessageAttachments)
		}
	}

	err = m.checkForEdits(receivedMessage)
	if err != nil {
		return err
	}

	err = m.checkForDeletes(receivedMessage)
	if err != nil {
		return err
	}

	err = m.checkForDeleteForMes(receivedMessage)
	if err != nil {
		return err
	}

	if !receivedMessage.Deleted && !receivedMessage.DeletedForMe {
		err = chat.UpdateFromMessage(receivedMessage, m.getTimesource())
		if err != nil {
			return err
		}
	}
	// Set in the modified maps chat
	state.Response.AddChat(chat)
	// TODO(samyoul) remove storing of an updated reference pointer?
	m.allChats.Store(chat.ID, chat)

	if !isSyncMessage && receivedMessage.EnsName != "" {
		oldRecord, err := m.ensVerifier.Add(contact.ID, receivedMessage.EnsName, receivedMessage.Clock)
		if err != nil {
			m.logger.Warn("failed to verify ENS name", zap.Error(err))
		} else if oldRecord == nil {
			// If oldRecord is nil, a new verification process will take place
			// so we reset the record
			contact.ENSVerified = false
			state.ModifiedContacts.Store(contact.ID, true)
			state.AllContacts.Store(contact.ID, contact)
		}
	}

	if !isSyncMessage && contact.DisplayName != receivedMessage.DisplayName && len(receivedMessage.DisplayName) != 0 {
		contact.DisplayName = receivedMessage.DisplayName
		state.ModifiedContacts.Store(contact.ID, true)
	}

	if receivedMessage.ContentType == protobuf.ChatMessage_COMMUNITY {
		m.logger.Debug("Handling community content type")

		communityResponse, err := m.handleWrappedCommunityDescriptionMessage(receivedMessage.GetCommunity())
		if err != nil {
			return err
		}
		community := communityResponse.Community
		receivedMessage.CommunityID = community.IDString()

		state.Response.AddCommunity(community)
		state.Response.CommunityChanges = append(state.Response.CommunityChanges, communityResponse.Changes)
	}

	receivedMessage.New = true
	state.Response.AddMessage(receivedMessage)

	return nil
}

func (m *Messenger) HandleChatMessage(state *ReceivedMessageState) error {
	return m.handleChatMessage(state, false)
}

func (m *Messenger) HandleImportedChatMessage(state *ReceivedMessageState) error {
	return m.handleChatMessage(state, true)
}

func (m *Messenger) addActivityCenterNotification(response *MessengerResponse, notification *ActivityCenterNotification) error {
	_, err := m.persistence.SaveActivityCenterNotification(notification, true)
	if err != nil {
		m.logger.Error("failed to save notification", zap.Error(err))
		return err
	}

	err = m.syncActivityCenterNotifications([]*ActivityCenterNotification{notification})
	if err != nil {
		m.logger.Error("addActivityCenterNotification, failed to sync activity center notifications", zap.Error(err))
		return err
	}

	state, err := m.persistence.GetActivityCenterState()
	if err != nil {
		m.logger.Error("failed to obtain activity center state", zap.Error(err))
		return err
	}
	response.AddActivityCenterNotification(notification)
	response.SetActivityCenterState(state)

	if !notification.Read {
		return m.syncActivityCenterNotificationState(state)
	}
	return nil
}

func (m *Messenger) HandleRequestAddressForTransaction(messageState *ReceivedMessageState, command protobuf.RequestAddressForTransaction) error {
	err := ValidateReceivedRequestAddressForTransaction(&command, messageState.CurrentMessageState.WhisperTimestamp)
	if err != nil {
		return err
	}
	message := &common.Message{
		ChatMessage: protobuf.ChatMessage{
			Clock:     command.Clock,
			Timestamp: messageState.CurrentMessageState.WhisperTimestamp,
			Text:      "Request address for transaction",
			// ChatId is only used as-is for messages sent to oneself (i.e: mostly sync) so no need to check it here
			ChatId:      command.GetChatId(),
			MessageType: protobuf.MessageType_ONE_TO_ONE,
			ContentType: protobuf.ChatMessage_TRANSACTION_COMMAND,
		},
		CommandParameters: &common.CommandParameters{
			ID:           messageState.CurrentMessageState.MessageID,
			Value:        command.Value,
			Contract:     command.Contract,
			CommandState: common.CommandStateRequestAddressForTransaction,
		},
	}
	return m.handleCommandMessage(messageState, message)
}

func (m *Messenger) handleSyncSetting(messageState *ReceivedMessageState, message *protobuf.SyncSetting) error {
	settingField, err := m.extractAndSaveSyncSetting(message)
	if err != nil {
		return err
	}

	if settingField == nil {
		return nil
	}

	switch message.GetType() {
	case protobuf.SyncSetting_DISPLAY_NAME:
		if newName := message.GetValueString(); newName != "" && m.account.Name != newName {
			m.account.Name = newName
			if err := m.multiAccounts.SaveAccount(*m.account); err != nil {
				return err
			}
		}
	case protobuf.SyncSetting_MNEMONIC_REMOVED:
		if message.GetValueBool() {
			if err := m.settings.DeleteMnemonic(); err != nil {
				return err
			}
			messageState.Response.AddSetting(&settings.SyncSettingField{SettingField: settings.Mnemonic})
		}
		return nil
	}
	messageState.Response.AddSetting(settingField)
	return nil
}

func (m *Messenger) handleSyncAccountCustomizationColor(state *ReceivedMessageState, message protobuf.SyncAccountCustomizationColor) error {
	err := m.multiAccounts.UpdateAccountCustomizationColor(message.GetKeyUid(), message.GetCustomizationColor(), message.GetUpdatedAt())
	if err != nil {
		return err
	}

	state.Response.CustomizationColor = message.GetCustomizationColor()
	return nil
}

func (m *Messenger) HandleRequestTransaction(messageState *ReceivedMessageState, command protobuf.RequestTransaction) error {
	err := ValidateReceivedRequestTransaction(&command, messageState.CurrentMessageState.WhisperTimestamp)
	if err != nil {
		return err
	}
	message := &common.Message{
		ChatMessage: protobuf.ChatMessage{
			Clock:     command.Clock,
			Timestamp: messageState.CurrentMessageState.WhisperTimestamp,
			Text:      "Request transaction",
			// ChatId is only used for messages sent to oneself (i.e: mostly sync) so no need to check it here
			ChatId:      command.GetChatId(),
			MessageType: protobuf.MessageType_ONE_TO_ONE,
			ContentType: protobuf.ChatMessage_TRANSACTION_COMMAND,
		},
		CommandParameters: &common.CommandParameters{
			ID:           messageState.CurrentMessageState.MessageID,
			Value:        command.Value,
			Contract:     command.Contract,
			CommandState: common.CommandStateRequestTransaction,
			Address:      command.Address,
		},
	}
	return m.handleCommandMessage(messageState, message)
}

func (m *Messenger) HandleAcceptRequestAddressForTransaction(messageState *ReceivedMessageState, command protobuf.AcceptRequestAddressForTransaction) error {
	err := ValidateReceivedAcceptRequestAddressForTransaction(&command, messageState.CurrentMessageState.WhisperTimestamp)
	if err != nil {
		return err
	}
	initialMessage, err := m.persistence.MessageByID(command.Id)
	if err != nil {
		return err
	}
	if initialMessage == nil {
		return errors.New("message not found")
	}

	if initialMessage.LocalChatID != messageState.CurrentMessageState.Contact.ID {
		return errors.New("From must match")
	}

	if initialMessage.OutgoingStatus == "" {
		return errors.New("Initial message must originate from us")
	}

	if initialMessage.CommandParameters.CommandState != common.CommandStateRequestAddressForTransaction {
		return errors.New("Wrong state for command")
	}

	initialMessage.Clock = command.Clock
	initialMessage.Timestamp = messageState.CurrentMessageState.WhisperTimestamp
	initialMessage.Text = requestAddressForTransactionAcceptedMessage
	initialMessage.CommandParameters.Address = command.Address
	initialMessage.Seen = false
	initialMessage.CommandParameters.CommandState = common.CommandStateRequestAddressForTransactionAccepted
	initialMessage.ChatId = command.GetChatId()

	// Hide previous message
	previousMessage, err := m.persistence.MessageByCommandID(messageState.CurrentMessageState.Contact.ID, command.Id)
	if err != nil && err != common.ErrRecordNotFound {
		return err
	}

	if previousMessage != nil {
		err = m.persistence.HideMessage(previousMessage.ID)
		if err != nil {
			return err
		}

		initialMessage.Replace = previousMessage.ID
	}

	return m.handleCommandMessage(messageState, initialMessage)
}

func (m *Messenger) HandleSendTransaction(messageState *ReceivedMessageState, command protobuf.SendTransaction) error {
	err := ValidateReceivedSendTransaction(&command, messageState.CurrentMessageState.WhisperTimestamp)
	if err != nil {
		return err
	}
	transactionToValidate := &TransactionToValidate{
		MessageID:       messageState.CurrentMessageState.MessageID,
		CommandID:       command.Id,
		TransactionHash: command.TransactionHash,
		FirstSeen:       messageState.CurrentMessageState.WhisperTimestamp,
		Signature:       command.Signature,
		Validate:        true,
		From:            messageState.CurrentMessageState.PublicKey,
		RetryCount:      0,
	}
	m.logger.Info("Saving transction to validate", zap.Any("transaction", transactionToValidate))

	return m.persistence.SaveTransactionToValidate(transactionToValidate)
}

func (m *Messenger) HandleDeclineRequestAddressForTransaction(messageState *ReceivedMessageState, command protobuf.DeclineRequestAddressForTransaction) error {
	err := ValidateReceivedDeclineRequestAddressForTransaction(&command, messageState.CurrentMessageState.WhisperTimestamp)
	if err != nil {
		return err
	}
	oldMessage, err := m.persistence.MessageByID(command.Id)
	if err != nil {
		return err
	}
	if oldMessage == nil {
		return errors.New("message not found")
	}

	if oldMessage.LocalChatID != messageState.CurrentMessageState.Contact.ID {
		return errors.New("From must match")
	}

	if oldMessage.OutgoingStatus == "" {
		return errors.New("Initial message must originate from us")
	}

	if oldMessage.CommandParameters.CommandState != common.CommandStateRequestAddressForTransaction {
		return errors.New("Wrong state for command")
	}

	oldMessage.Clock = command.Clock
	oldMessage.Timestamp = messageState.CurrentMessageState.WhisperTimestamp
	oldMessage.Text = requestAddressForTransactionDeclinedMessage
	oldMessage.Seen = false
	oldMessage.CommandParameters.CommandState = common.CommandStateRequestAddressForTransactionDeclined
	oldMessage.ChatId = command.GetChatId()

	// Hide previous message
	err = m.persistence.HideMessage(command.Id)
	if err != nil {
		return err
	}
	oldMessage.Replace = command.Id

	return m.handleCommandMessage(messageState, oldMessage)
}

func (m *Messenger) HandleDeclineRequestTransaction(messageState *ReceivedMessageState, command protobuf.DeclineRequestTransaction) error {
	err := ValidateReceivedDeclineRequestTransaction(&command, messageState.CurrentMessageState.WhisperTimestamp)
	if err != nil {
		return err
	}
	oldMessage, err := m.persistence.MessageByID(command.Id)
	if err != nil {
		return err
	}
	if oldMessage == nil {
		return errors.New("message not found")
	}

	if oldMessage.LocalChatID != messageState.CurrentMessageState.Contact.ID {
		return errors.New("From must match")
	}

	if oldMessage.OutgoingStatus == "" {
		return errors.New("Initial message must originate from us")
	}

	if oldMessage.CommandParameters.CommandState != common.CommandStateRequestTransaction {
		return errors.New("Wrong state for command")
	}

	oldMessage.Clock = command.Clock
	oldMessage.Timestamp = messageState.CurrentMessageState.WhisperTimestamp
	oldMessage.Text = transactionRequestDeclinedMessage
	oldMessage.Seen = false
	oldMessage.CommandParameters.CommandState = common.CommandStateRequestTransactionDeclined
	oldMessage.ChatId = command.GetChatId()

	// Hide previous message
	err = m.persistence.HideMessage(command.Id)
	if err != nil {
		return err
	}
	oldMessage.Replace = command.Id

	return m.handleCommandMessage(messageState, oldMessage)
}

func (m *Messenger) matchChatEntity(chatEntity common.ChatEntity) (*Chat, error) {
	if chatEntity.GetSigPubKey() == nil {
		m.logger.Error("public key can't be empty")
		return nil, errors.New("received a chatEntity with empty public key")
	}

	switch {
	case chatEntity.GetMessageType() == protobuf.MessageType_PUBLIC_GROUP:
		// For public messages, all outgoing and incoming messages have the same chatID
		// equal to a public chat name.
		chatID := chatEntity.GetChatId()
		chat, ok := m.allChats.Load(chatID)
		if !ok {
			return nil, errors.New("received a public chatEntity from non-existing chat")
		}
		if !chat.Public() && !chat.ProfileUpdates() && !chat.Timeline() {
			return nil, ErrMessageForWrongChatType
		}
		return chat, nil
	case chatEntity.GetMessageType() == protobuf.MessageType_ONE_TO_ONE && common.IsPubKeyEqual(chatEntity.GetSigPubKey(), &m.identity.PublicKey):
		// It's a private message coming from us so we rely on Message.ChatID
		// If chat does not exist, it should be created to support multidevice synchronization.
		chatID := chatEntity.GetChatId()
		chat, ok := m.allChats.Load(chatID)
		if !ok {
			if len(chatID) != PubKeyStringLength {
				return nil, errors.New("invalid pubkey length")
			}
			bytePubKey, err := hex.DecodeString(chatID[2:])
			if err != nil {
				return nil, errors.Wrap(err, "failed to decode hex chatID")
			}

			pubKey, err := crypto.UnmarshalPubkey(bytePubKey)
			if err != nil {
				return nil, errors.Wrap(err, "failed to decode pubkey")
			}

			chat = CreateOneToOneChat(chatID[:8], pubKey, m.getTimesource())
		}
		// if we are the sender, the chat must be active
		chat.Active = true
		return chat, nil
	case chatEntity.GetMessageType() == protobuf.MessageType_ONE_TO_ONE:
		// It's an incoming private chatEntity. ChatID is calculated from the signature.
		// If a chat does not exist, a new one is created and saved.
		chatID := contactIDFromPublicKey(chatEntity.GetSigPubKey())
		chat, ok := m.allChats.Load(chatID)
		if !ok {
			// TODO: this should be a three-word name used in the mobile client
			chat = CreateOneToOneChat(chatID[:8], chatEntity.GetSigPubKey(), m.getTimesource())
			chat.Active = false
		}
		// We set the chat as inactive and will create a notification
		// if it's not coming from a contact
		contact, ok := m.allContacts.Load(chatID)
		chat.Active = chat.Active || (ok && contact.added())
		return chat, nil
	case chatEntity.GetMessageType() == protobuf.MessageType_COMMUNITY_CHAT:
		chatID := chatEntity.GetChatId()
		chat, ok := m.allChats.Load(chatID)
		if !ok {
			return nil, errors.New("received community chat chatEntity for non-existing chat")
		}

		if chat.CommunityID == "" || chat.ChatType != ChatTypeCommunityChat {
			return nil, errors.New("not an community chat")
		}

		var emojiReaction bool
		var pinMessage bool
		// We allow emoji reactions from anyone
		switch chatEntity.(type) {
		case *EmojiReaction:
			emojiReaction = true
		case *common.PinMessage:
			pinMessage = true
		}

		canPost, err := m.communitiesManager.CanPost(chatEntity.GetSigPubKey(), chat.CommunityID, chat.CommunityChatID(), chatEntity.GetGrant())
		if err != nil {
			return nil, err
		}

		community, err := m.communitiesManager.GetByIDString(chat.CommunityID)
		if err != nil {
			return nil, err
		}

		isMemberOwnerOrAdmin := community.IsMemberOwnerOrAdmin(chatEntity.GetSigPubKey())
		pinMessageAllowed := community.AllowsAllMembersToPinMessage()

		if (pinMessage && !isMemberOwnerOrAdmin && !pinMessageAllowed) || (!emojiReaction && !canPost) {
			return nil, errors.New("user can't post")
		}

		return chat, nil
	case chatEntity.GetMessageType() == protobuf.MessageType_PRIVATE_GROUP:
		// In the case of a group chatEntity, ChatID is the same for all messages belonging to a group.
		// It needs to be verified if the signature public key belongs to the chat.
		chatID := chatEntity.GetChatId()
		chat, ok := m.allChats.Load(chatID)
		if !ok {
			return nil, errors.New("received group chat chatEntity for non-existing chat")
		}

		senderKeyHex := contactIDFromPublicKey(chatEntity.GetSigPubKey())
		myKeyHex := contactIDFromPublicKey(&m.identity.PublicKey)
		senderIsMember := false
		iAmMember := false
		for _, member := range chat.Members {
			if member.ID == senderKeyHex {
				senderIsMember = true
			}
			if member.ID == myKeyHex {
				iAmMember = true
			}
		}

		if senderIsMember && iAmMember {
			return chat, nil
		}

		return nil, errors.New("did not find a matching group chat")
	default:
		return nil, errors.New("can not match a chat because there is no valid case")
	}
}

func (m *Messenger) messageExists(messageID string, existingMessagesMap map[string]bool) (bool, error) {
	if _, ok := existingMessagesMap[messageID]; ok {
		return true, nil
	}

	existingMessagesMap[messageID] = true

	// Check against the database, this is probably a bit slow for
	// each message, but for now might do, we'll make it faster later
	existingMessage, err := m.persistence.MessageByID(messageID)
	if err != nil && err != common.ErrRecordNotFound {
		return false, err
	}
	if existingMessage != nil {
		return true, nil
	}
	return false, nil
}

func (m *Messenger) HandleEmojiReaction(state *ReceivedMessageState, pbEmojiR protobuf.EmojiReaction) error {
	logger := m.logger.With(zap.String("site", "HandleEmojiReaction"))
	if err := ValidateReceivedEmojiReaction(&pbEmojiR, state.Timesource.GetCurrentTime()); err != nil {
		logger.Error("invalid emoji reaction", zap.Error(err))
		return err
	}

	from := state.CurrentMessageState.Contact.ID

	emojiReaction := &EmojiReaction{
		EmojiReaction: pbEmojiR,
		From:          from,
		SigPubKey:     state.CurrentMessageState.PublicKey,
	}

	existingEmoji, err := m.persistence.EmojiReactionByID(emojiReaction.ID())
	if err != common.ErrRecordNotFound && err != nil {
		return err
	}

	if existingEmoji != nil && existingEmoji.Clock >= pbEmojiR.Clock {
		// this is not a valid emoji, ignoring
		return nil
	}

	chat, err := m.matchChatEntity(emojiReaction)
	if err != nil {
		return err // matchChatEntity returns a descriptive error message
	}

	// Set local chat id
	emojiReaction.LocalChatID = chat.ID

	logger.Debug("Handling emoji reaction")

	if chat.LastClockValue < pbEmojiR.Clock {
		chat.LastClockValue = pbEmojiR.Clock
	}

	state.Response.AddChat(chat)
	// TODO(samyoul) remove storing of an updated reference pointer?
	state.AllChats.Store(chat.ID, chat)

	// save emoji reaction
	err = m.persistence.SaveEmojiReaction(emojiReaction)
	if err != nil {
		return err
	}

	state.EmojiReactions[emojiReaction.ID()] = emojiReaction

	return nil
}

func (m *Messenger) HandleGroupChatInvitation(state *ReceivedMessageState, pbGHInvitations protobuf.GroupChatInvitation) error {
	allowed, err := m.isMessageAllowedFrom(state.CurrentMessageState.Contact.ID, nil)
	if err != nil {
		return err
	}

	if !allowed {
		return ErrMessageNotAllowed
	}
	logger := m.logger.With(zap.String("site", "HandleGroupChatInvitation"))
	if err := ValidateReceivedGroupChatInvitation(&pbGHInvitations); err != nil {
		logger.Error("invalid group chat invitation", zap.Error(err))
		return err
	}

	groupChatInvitation := &GroupChatInvitation{
		GroupChatInvitation: pbGHInvitations,
		SigPubKey:           state.CurrentMessageState.PublicKey,
	}

	//From is the PK of author of invitation request
	if groupChatInvitation.State == protobuf.GroupChatInvitation_REJECTED {
		//rejected so From is the current user who received this rejection
		groupChatInvitation.From = types.EncodeHex(crypto.FromECDSAPub(&m.identity.PublicKey))
	} else {
		//invitation request, so From is the author of message
		groupChatInvitation.From = state.CurrentMessageState.Contact.ID
	}

	existingInvitation, err := m.persistence.InvitationByID(groupChatInvitation.ID())
	if err != common.ErrRecordNotFound && err != nil {
		return err
	}

	if existingInvitation != nil && existingInvitation.Clock >= pbGHInvitations.Clock {
		// this is not a valid invitation, ignoring
		return nil
	}

	// save invitation
	err = m.persistence.SaveInvitation(groupChatInvitation)
	if err != nil {
		return err
	}

	state.GroupChatInvitations[groupChatInvitation.ID()] = groupChatInvitation

	return nil
}

// HandleChatIdentity handles an incoming protobuf.ChatIdentity
// extracts contact information stored in the protobuf and adds it to the user's contact for update.
func (m *Messenger) HandleChatIdentity(state *ReceivedMessageState, ci protobuf.ChatIdentity) error {
	s, err := m.settings.GetSettings()
	if err != nil {
		return err
	}

	contact := state.CurrentMessageState.Contact
	viewFromContacts := s.ProfilePicturesVisibility == settings.ProfilePicturesVisibilityContactsOnly
	viewFromNoOne := s.ProfilePicturesVisibility == settings.ProfilePicturesVisibilityNone

	m.logger.Debug("settings found",
		zap.Bool("viewFromContacts", viewFromContacts),
		zap.Bool("viewFromNoOne", viewFromNoOne),
	)

	// If we don't want to view profile images from anyone, don't process identity images.
	// We don't want to store the profile images of other users, even if we don't display images.
	inOurContacts, ok := m.allContacts.Load(state.CurrentMessageState.Contact.ID)

	isContact := ok && inOurContacts.added()
	if viewFromNoOne && !isContact {
		return nil
	}

	// If there are no images attached to a ChatIdentity, check if message is allowed
	// Or if there are images and visibility is set to from contacts only, check if message is allowed
	// otherwise process the images without checking if the message is allowed
	if len(ci.Images) == 0 || (len(ci.Images) > 0 && (viewFromContacts)) {
		allowed, err := m.isMessageAllowedFrom(state.CurrentMessageState.Contact.ID, nil)
		if err != nil {
			return err
		}

		if !allowed {
			return ErrMessageNotAllowed
		}
	}

	err = DecryptIdentityImagesWithIdentityPrivateKey(ci.Images, m.identity, state.CurrentMessageState.PublicKey)
	if err != nil {
		return err
	}

	// Remove any images still encrypted after the decryption process
	for name, image := range ci.Images {
		if image.Encrypted {
			delete(ci.Images, name)
		}
	}

	clockChanged, imagesChanged, err := m.persistence.SaveContactChatIdentity(contact.ID, &ci)
	if err != nil {
		return err
	}
	contactModified := false

	if imagesChanged {
		for imageType, image := range ci.Images {
			if contact.Images == nil {
				contact.Images = make(map[string]images.IdentityImage)
			}
			contact.Images[imageType] = images.IdentityImage{Name: imageType, Payload: image.Payload, Clock: ci.Clock}

		}
		if err = m.updateContactImagesURL(contact); err != nil {
			return err
		}

		contactModified = true
	}

	if clockChanged {
		if err = ValidateDisplayName(&ci.DisplayName); err != nil {
			return err
		}

		if contact.DisplayName != ci.DisplayName && len(ci.DisplayName) != 0 {
			contact.DisplayName = ci.DisplayName
			contactModified = true
		}

		if err = ValidateBio(&ci.Description); err != nil {
			return err
		}

		if contact.Bio != ci.Description {
			contact.Bio = ci.Description
			contactModified = true
		}

		socialLinks := identity.NewSocialLinks(ci.SocialLinks)
		if err = ValidateSocialLinks(socialLinks); err != nil {
			return err
		}

		if !contact.SocialLinks.Equal(socialLinks) {
			contact.SocialLinks = socialLinks
			contactModified = true
		}
	}

	if contactModified {
		state.ModifiedContacts.Store(contact.ID, true)
		state.AllContacts.Store(contact.ID, contact)
	}

	return nil
}

func (m *Messenger) HandleAnonymousMetricBatch(amb protobuf.AnonymousMetricBatch) error {

	// TODO
	return nil
}

func (m *Messenger) checkForEdits(message *common.Message) error {
	// Check for any pending edit
	// If any pending edits are available and valid, apply them
	edits, err := m.persistence.GetEdits(message.ID, message.From)
	if err != nil {
		return err
	}

	if len(edits) == 0 {
		return nil
	}

	// Apply the first edit that is valid
	for _, e := range edits {
		if e.Clock >= message.Clock {
			// Update message and return it
			err := m.applyEditMessage(&e.EditMessage, message)
			if err != nil {
				return err
			}
			return nil
		}
	}

	return nil
}

func (m *Messenger) getMessagesToCheckForDelete(message *common.Message) ([]*common.Message, error) {
	var messagesToCheck []*common.Message
	if message.ContentType == protobuf.ChatMessage_IMAGE {
		image := message.GetImage()
		if image != nil && image.AlbumId != "" {
			messagesInTheAlbum, err := m.persistence.albumMessages(message.ChatId, image.GetAlbumId())
			if err != nil {
				return nil, err
			}
			messagesToCheck = append(messagesToCheck, messagesInTheAlbum...)
		}
	}
	messagesToCheck = append(messagesToCheck, message)
	return messagesToCheck, nil
}

func (m *Messenger) checkForDeletes(message *common.Message) error {
	// Get all messages part of the album
	messagesToCheck, err := m.getMessagesToCheckForDelete(message)
	if err != nil {
		return err
	}

	var messageDeletes []*DeleteMessage
	applyDelete := false
	// Loop all messages part of the album, if one of them is marked as deleted, we delete them all
	for _, messageToCheck := range messagesToCheck {
		// Check for any pending deletes
		// If any pending deletes are available and valid, apply them
		messageDeletes, err = m.persistence.GetDeletes(messageToCheck.ID, messageToCheck.From)
		if err != nil {
			return err
		}

		if len(messageDeletes) == 0 {
			continue
		}
		// Once one messageDelete has been found, we apply it to all the images in the album
		applyDelete = true
		break
	}
	if applyDelete {
		for _, messageToCheck := range messagesToCheck {
			err := m.applyDeleteMessage(messageDeletes, messageToCheck)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Messenger) checkForDeleteForMes(message *common.Message) error {
	messagesToCheck, err := m.getMessagesToCheckForDelete(message)
	if err != nil {
		return err
	}

	var messageDeleteForMes []*protobuf.DeleteForMeMessage
	applyDelete := false
	for _, messageToCheck := range messagesToCheck {
		if !applyDelete {
			// Check for any pending delete for mes
			// If any pending deletes are available and valid, apply them
			messageDeleteForMes, err = m.persistence.GetDeleteForMeMessagesByMessageID(messageToCheck.ID)
			if err != nil {
				return err
			}

			if len(messageDeleteForMes) == 0 {
				continue
			}
		}
		// Once one messageDeleteForMes has been found, we apply it to all the images in the album
		applyDelete = true

		err := m.applyDeleteForMeMessage(messageToCheck)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Messenger) isMessageAllowedFrom(publicKey string, chat *Chat) (bool, error) {
	onlyFromContacts, err := m.settings.GetMessagesFromContactsOnly()
	if err != nil {
		return false, err
	}

	if !onlyFromContacts {
		return true, nil
	}

	// if it's from us, it's allowed
	if m.myHexIdentity() == publicKey {
		return true, nil
	}

	// If the chat is active, we allow it
	if chat != nil && chat.Active {
		return true, nil
	}

	// If the chat is public, we allow it
	if chat != nil && chat.Public() {
		return true, nil
	}

	contact, ok := m.allContacts.Load(publicKey)
	if !ok {
		// If it's not in contacts, we don't allow it
		return false, nil
	}

	// Otherwise we check if we added it
	return contact.added(), nil
}

func (m *Messenger) updateUnviewedCounts(chat *Chat, mentionedOrReplied bool) {
	chat.UnviewedMessagesCount++
	if mentionedOrReplied {
		chat.UnviewedMentionsCount++
	}
}

func mapSyncAccountToAccount(message *protobuf.SyncAccount, accountOperability accounts.AccountOperable, accType accounts.AccountType) *accounts.Account {
	return &accounts.Account{
		Address:   types.BytesToAddress(message.Address),
		KeyUID:    message.KeyUid,
		PublicKey: types.HexBytes(message.PublicKey),
		Type:      accType,
		Path:      message.Path,
		Name:      message.Name,
		ColorID:   multiaccountscommon.CustomizationColor(message.ColorId),
		Emoji:     message.Emoji,
		Wallet:    message.Wallet,
		Chat:      message.Chat,
		Hidden:    message.Hidden,
		Clock:     message.Clock,
		Operable:  accountOperability,
		Removed:   message.Removed,
		Position:  message.Position,
	}
}

func (m *Messenger) resolveAccountOperability(syncAcc *protobuf.SyncAccount, syncKpMigratedToKeycard bool, accountReceivedFromLocalPairing bool) (accounts.AccountOperable, error) {
	accountsOperability := accounts.AccountNonOperable
	dbAccount, err := m.settings.GetAccountByAddress(types.BytesToAddress(syncAcc.Address))
	if err != nil && err != accounts.ErrDbAccountNotFound {
		return accountsOperability, err
	}
	if dbAccount != nil {
		return dbAccount.Operable, nil
	}

	if !syncKpMigratedToKeycard {
		// We're here when we receive a keypair from the paired device which is either:
		// 1. regular keypair or
		// 2. was just converted from keycard to a regular keypair.
		dbKeycardsForKeyUID, err := m.settings.GetKeycardsWithSameKeyUID(syncAcc.KeyUid)
		if err != nil {
			return accounts.AccountNonOperable, err
		}

		if len(dbKeycardsForKeyUID) > 0 {
			// We're here in case 2. from above and in this case we need to mark all accounts for this keypair non operable
			return accounts.AccountNonOperable, nil
		}
	}

	if syncKpMigratedToKeycard || accountReceivedFromLocalPairing || syncAcc.Chat || syncAcc.Wallet {
		accountsOperability = accounts.AccountFullyOperable
	} else {
		partiallyOrFullyOperable, err := m.settings.IsAnyAccountPartiallyOrFullyOperableForKeyUID(syncAcc.KeyUid)
		if err != nil {
			if err == accounts.ErrDbKeypairNotFound {
				return accounts.AccountNonOperable, nil
			}
			return accounts.AccountNonOperable, err
		}
		if partiallyOrFullyOperable {
			accountsOperability = accounts.AccountPartiallyOperable
		}
	}

	return accountsOperability, nil
}

func (m *Messenger) handleSyncWatchOnlyAccount(message *protobuf.SyncAccount) (*accounts.Account, error) {
	if message.KeyUid != "" {
		return nil, ErrNotWatchOnlyAccount
	}

	accountOperability := accounts.AccountFullyOperable

	accAddress := types.BytesToAddress(message.Address)
	dbAccount, err := m.settings.GetAccountByAddress(accAddress)
	if err != nil && err != accounts.ErrDbAccountNotFound {
		return nil, err
	}

	if dbAccount != nil {
		if message.Clock <= dbAccount.Clock {
			return nil, ErrTryingToStoreOldWalletAccount
		}

		if message.Removed {
			err = m.settings.DeleteAccount(accAddress, message.Clock)
			if err != nil {
				return nil, err
			}
			err = m.settings.ResolveAccountsPositions(message.Clock)
			dbAccount.Removed = true
			return dbAccount, err
		}
	} else {
		if message.Removed {
			return nil, ErrTryingToRemoveUnexistingWalletAccount
		}
	}

	acc := mapSyncAccountToAccount(message, accountOperability, accounts.AccountTypeWatch)

	err = m.settings.SaveOrUpdateAccounts([]*accounts.Account{acc}, false)
	if err != nil {
		return nil, err
	}

	return acc, nil
}

func (m *Messenger) handleSyncAccountsPositions(message *protobuf.SyncAccountsPositions) ([]*accounts.Account, error) {
	if len(message.Accounts) == 0 {
		return nil, nil
	}

	dbLastUpdate, err := m.settings.GetClockOfLastAccountsPositionChange()
	if err != nil {
		return nil, err
	}

	// Since adding new account updates `ClockOfLastAccountsPositionChange` we should handle account order changes
	// even they are with the same clock, that ensures the correct order in case of syncing devices.
	if message.Clock < dbLastUpdate {
		return nil, ErrTryingToApplyOldWalletAccountsOrder
	}

	var accs []*accounts.Account
	for _, sAcc := range message.Accounts {
		acc := &accounts.Account{
			Address:  types.BytesToAddress(sAcc.Address),
			KeyUID:   sAcc.KeyUid,
			Position: sAcc.Position,
		}
		accs = append(accs, acc)
	}

	err = m.settings.SetWalletAccountsPositions(accs, message.Clock)
	if err != nil {
		return nil, err
	}

	return accs, nil
}

func (m *Messenger) handleSyncKeypair(message *protobuf.SyncKeypair) (*accounts.Keypair, error) {
	if message == nil {
		return nil, errors.New("handleSyncKeypair receive a nil message")
	}
	dbKeypair, err := m.settings.GetKeypairByKeyUID(message.KeyUid)
	if err != nil && err != accounts.ErrDbKeypairNotFound {
		return nil, err
	}

	kp := &accounts.Keypair{
		KeyUID:                  message.KeyUid,
		Name:                    message.Name,
		Type:                    accounts.KeypairType(message.Type),
		DerivedFrom:             message.DerivedFrom,
		LastUsedDerivationIndex: message.LastUsedDerivationIndex,
		SyncedFrom:              message.SyncedFrom,
		Clock:                   message.Clock,
		Removed:                 message.Removed,
	}

	accountReceivedFromLocalPairing := message.SyncedFrom == accounts.SyncedFromLocalPairing
	if dbKeypair != nil {
		if dbKeypair.Clock >= kp.Clock {
			return nil, ErrTryingToStoreOldKeypair
		}
		// in case of keypair update, we need to keep `synced_from` field as it was when keypair was introduced to this device for the first time
		kp.SyncedFrom = dbKeypair.SyncedFrom
	} else {
		if kp.Removed {
			return nil, nil
		}
	}

	for _, sAcc := range message.Accounts {
		syncKpMigratedToKeycard := len(message.Keycards) > 0
		accountOperability, err := m.resolveAccountOperability(sAcc, syncKpMigratedToKeycard, accountReceivedFromLocalPairing)
		if err != nil {
			return nil, err
		}
		acc := mapSyncAccountToAccount(sAcc, accountOperability, accounts.GetAccountTypeForKeypairType(kp.Type))

		kp.Accounts = append(kp.Accounts, acc)
	}

	if kp.Removed {
		// delete all keystore files
		for _, dbAcc := range dbKeypair.Accounts {
			err = m.deleteKeystoreFileForAddress(dbAcc.Address)
			if err != nil {
				return nil, err
			}
		}
	} else if !accountReceivedFromLocalPairing && dbKeypair != nil {
		for _, dbAcc := range dbKeypair.Accounts {
			found := false
			for _, acc := range kp.Accounts {
				if dbAcc.Address == acc.Address {
					found = true
					break
				}
			}
			if !found {
				err = m.deleteKeystoreFileForAddress(dbAcc.Address)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	err = m.settings.DeleteKeypair(message.KeyUid) // deleting keypair will delete related keycards as well
	if err != nil && err != accounts.ErrDbKeypairNotFound {
		return nil, err
	}

	// if entire keypair was removed, there is no point to continue
	if kp.Removed {
		err = m.settings.ResolveAccountsPositions(message.Clock)
		if err != nil {
			return nil, err
		}
		return kp, nil
	}

	// save keypair first
	err = m.settings.SaveOrUpdateKeypair(kp)
	if err != nil {
		return nil, err
	}

	// then resolve accounts positions, cause some accounts might be removed
	err = m.settings.ResolveAccountsPositions(message.Clock)
	if err != nil {
		return nil, err
	}

	for _, sKc := range message.Keycards {
		kc := accounts.Keycard{}
		kc.FromSyncKeycard(sKc)
		err = m.settings.SaveOrUpdateKeycard(kc, message.Clock, false)
		if err != nil {
			return nil, err
		}
		kp.Keycards = append(kp.Keycards, &kc)
	}

	// getting keypair form the db, cause keypair related accounts positions might be changed
	dbKeypair, err = m.settings.GetKeypairByKeyUID(message.KeyUid)
	if err != nil {
		return nil, err
	}
	return dbKeypair, nil
}

func (m *Messenger) HandleSyncAccountsPositions(state *ReceivedMessageState, message protobuf.SyncAccountsPositions) error {
	accs, err := m.handleSyncAccountsPositions(&message)
	if err != nil {
		if err == ErrTryingToApplyOldWalletAccountsOrder ||
			err == accounts.ErrAccountWrongPosition ||
			err == accounts.ErrNotTheSameNumberOdAccountsToApplyReordering ||
			err == accounts.ErrNotTheSameAccountsToApplyReordering {
			m.logger.Warn("syncing accounts order issue", zap.Error(err))
			return nil
		}
		return err
	}

	state.Response.AccountsPositions = append(state.Response.AccountsPositions, accs...)

	return nil
}

func (m *Messenger) HandleSyncWatchOnlyAccount(state *ReceivedMessageState, message protobuf.SyncAccount) error {
	acc, err := m.handleSyncWatchOnlyAccount(&message)
	if err != nil {
		if err == ErrTryingToStoreOldWalletAccount {
			return nil
		}
		return err
	}

	state.Response.WatchOnlyAccounts = append(state.Response.WatchOnlyAccounts, acc)

	return nil
}

func (m *Messenger) HandleSyncKeypair(state *ReceivedMessageState, message protobuf.SyncKeypair) error {
	kp, err := m.handleSyncKeypair(&message)
	if err != nil {
		if err == ErrTryingToStoreOldKeypair {
			return nil
		}
		return err
	}

	state.Response.Keypairs = append(state.Response.Keypairs, kp)

	return nil
}

func (m *Messenger) HandleSyncContactRequestDecision(state *ReceivedMessageState, message protobuf.SyncContactRequestDecision) error {
	var err error
	var response *MessengerResponse

	if message.DecisionStatus == protobuf.SyncContactRequestDecision_ACCEPTED {
		response, err = m.updateAcceptedContactRequest(nil, message.RequestId)
	} else {
		response, err = m.declineContactRequest(message.RequestId, true)
	}
	if err != nil {
		return err
	}

	state.Response = response

	return nil
}
