package protobuf

import (
	"github.com/golang/protobuf/proto"
)

//go:generate protoc --go_out=. ./chat_message.proto ./application_metadata_message.proto ./membership_update_message.proto ./command.proto ./contact.proto ./pairing.proto ./push_notifications.proto ./emoji_reaction.proto ./enums.proto ./group_chat_invitation.proto ./chat_identity.proto ./communities.proto ./pin_message.proto ./anon_metrics.proto ./status_update.proto ./sync_settings.proto ./contact_verification.proto ./community_update.proto ./url_data.proto

func Unmarshal(payload []byte) (*ApplicationMetadataMessage, error) {
	var message ApplicationMetadataMessage
	err := proto.Unmarshal(payload, &message)
	if err != nil {
		return nil, err
	}

	return &message, nil
}
