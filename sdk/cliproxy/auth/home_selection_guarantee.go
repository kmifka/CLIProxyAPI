package auth

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
)

type homeSelectionBinding struct {
	AuthIndex            string `json:"auth_index"`
	RegistrationLifetime string `json:"registration_lifetime"`
	StateVersion         *int64 `json:"state_version"`
	MetadataDigest       string `json:"metadata_digest"`
	WireMetadataDigest   string `json:"wire_metadata_digest"`
}
type homeSelectionGuarantee struct {
	Schema         int    `json:"schema"`
	RequestedModel string `json:"requested_model"`
	RequestedTier  string `json:"requested_tier"`
	homeSelectionBinding
}

func strictHomeSelectionJSON(raw json.RawMessage, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func validateHomeSelectionGuarantee(dispatch *homeAuthDispatchResponse, requestedModel string) error {
	deny := func() error {
		return &Error{Code: "auth_unavailable", Message: "Home ultrafast selection guarantee unavailable", HTTPStatus: http.StatusServiceUnavailable}
	}
	var g homeSelectionGuarantee
	var b homeSelectionBinding
	if strictHomeSelectionJSON(dispatch.SelectionGuarantee, &g) != nil || strictHomeSelectionJSON(dispatch.SelectionBinding, &b) != nil {
		return deny()
	}
	if g.Schema != 1 || g.RequestedModel != requestedModel || g.RequestedTier != "ultrafast" || b.AuthIndex == "" || b.AuthIndex != dispatch.AuthIndex || b.RegistrationLifetime == "" || b.StateVersion == nil || *b.StateVersion < 0 || g.StateVersion == nil || *g.StateVersion != *b.StateVersion || g.AuthIndex != b.AuthIndex || g.RegistrationLifetime != b.RegistrationLifetime || g.MetadataDigest != b.MetadataDigest || g.WireMetadataDigest != b.WireMetadataDigest || dispatch.Auth.ID == "" || dispatch.Auth.Provider != "codex" || dispatch.Provider != "codex" {
		return deny()
	}
	for _, digest := range []string{b.MetadataDigest, b.WireMetadataDigest} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != digest {
			return deny()
		}
	}
	// Hash the exact wire metadata projection, not a decoded float64 map. Full
	// private metadata and durable lifetime are protected Home envelope bindings.
	var rawAuth struct {
		Metadata json.RawMessage `json:"metadata"`
	}
	if json.Unmarshal(dispatch.RawAuth, &rawAuth) != nil {
		return deny()
	}
	var metadata map[string]json.RawMessage
	if len(rawAuth.Metadata) == 0 {
		rawAuth.Metadata = []byte("null")
	}
	if json.Unmarshal(rawAuth.Metadata, &metadata) != nil {
		return deny()
	}
	canonical, err := json.Marshal(metadata)
	if err != nil {
		return deny()
	}
	digest := sha256.Sum256(canonical)
	if hex.EncodeToString(digest[:]) != b.WireMetadataDigest {
		return deny()
	}
	return nil
}

// HomeUltrafastValidated is request-local, never decoded from caller metadata.
func (a *Auth) HomeUltrafastValidated() bool { return a != nil && a.homeUltrafastValidated }
