package dingtalk

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

const (
	dingTalkAttachmentImage = "image"
	dingTalkAttachmentFile  = "file"
	dingTalkImageMediaType  = "image"
	dingTalkFileMediaType   = "application/octet-stream"
)

// MediaContent is the SDK-neutral DingTalk image/file payload fragment.
type MediaContent struct {
	DownloadCode string `json:"downloadCode"`
	FileName     string `json:"fileName,omitempty"`
	MediaType    string `json:"mediaType,omitempty"`
}

// RichTextItem captures the media-bearing parts of DingTalk rich-text
// callbacks without importing the real SDK.
type RichTextItem struct {
	Type                string `json:"type,omitempty"`
	Text                string `json:"text,omitempty"`
	Content             string `json:"content,omitempty"`
	DownloadCode        string `json:"downloadCode,omitempty"`
	PictureDownloadCode string `json:"pictureDownloadCode,omitempty"`
	FileName            string `json:"fileName,omitempty"`
	MediaType           string `json:"mediaType,omitempty"`
}

// MediaDownloadRequest mirrors DingTalk's robot message file-download request
// shape while staying SDK-neutral.
type MediaDownloadRequest struct {
	RobotCode    string `json:"robotCode"`
	DownloadCode string `json:"downloadCode"`
}

// MediaDownloadResult mirrors the DingTalk response body field that carries a
// temporary media download URL.
type MediaDownloadResult struct {
	DownloadURL string `json:"downloadUrl"`
}

// MediaDownloadClient is the minimal real-SDK seam for resolving DingTalk
// image/file download codes into temporary download URLs.
type MediaDownloadClient interface {
	DownloadMedia(ctx context.Context, req MediaDownloadRequest) (MediaDownloadResult, error)
}

type mediaCandidate struct {
	kind         string
	mediaType    string
	downloadCode string
	fileName     string
}

func (b *Bot) mediaAttachments(ctx context.Context, msg InboundMessage) []gateway.Attachment {
	candidates := dingtalkMediaCandidates(msg)
	if len(candidates) == 0 {
		return nil
	}

	attachments := make([]gateway.Attachment, 0, len(candidates))
	for _, candidate := range candidates {
		att := gateway.Attachment{
			Kind:      candidate.kind,
			MediaType: candidate.mediaType,
			FileName:  candidate.fileName,
			SourceID:  candidate.downloadCode,
		}
		url, err := b.resolveMediaDownloadURL(ctx, candidate.downloadCode)
		if err != nil {
			att.URL = candidate.downloadCode
			att.Error = err.Error()
		} else {
			att.URL = url
		}
		attachments = append(attachments, att)
	}
	return attachments
}

func dingtalkMediaCandidates(msg InboundMessage) []mediaCandidate {
	var candidates []mediaCandidate
	if msg.ImageContent != nil {
		if code := strings.TrimSpace(msg.ImageContent.DownloadCode); code != "" {
			candidates = append(candidates, mediaCandidate{
				kind:         dingTalkAttachmentImage,
				mediaType:    mediaTypeOrDefault(msg.ImageContent.MediaType, dingTalkImageMediaType),
				downloadCode: code,
				fileName:     strings.TrimSpace(msg.ImageContent.FileName),
			})
		}
	}

	for _, item := range msg.RichTextContent {
		code := strings.TrimSpace(item.DownloadCode)
		if code == "" {
			code = strings.TrimSpace(item.PictureDownloadCode)
		}
		if code == "" {
			continue
		}
		kind, mediaType := dingtalkRichTextMediaRoute(item)
		candidates = append(candidates, mediaCandidate{
			kind:         kind,
			mediaType:    mediaTypeOrDefault(item.MediaType, mediaType),
			downloadCode: code,
			fileName:     strings.TrimSpace(item.FileName),
		})
	}
	return candidates
}

func dingtalkRichTextMediaRoute(item RichTextItem) (kind, mediaType string) {
	switch strings.ToLower(strings.TrimSpace(item.Type)) {
	case "image", "picture":
		return dingTalkAttachmentImage, dingTalkImageMediaType
	default:
		return dingTalkAttachmentFile, dingTalkFileMediaType
	}
}

func mediaTypeOrDefault(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func (b *Bot) resolveMediaDownloadURL(ctx context.Context, downloadCode string) (string, error) {
	downloadCode = strings.TrimSpace(downloadCode)
	if downloadCode == "" {
		return "", errors.New("dingtalk: media download code is required")
	}
	client, ok := b.client.(MediaDownloadClient)
	if !ok || client == nil {
		return "", errors.New("dingtalk: media download client unavailable")
	}
	result, err := client.DownloadMedia(ctx, MediaDownloadRequest{
		RobotCode:    strings.TrimSpace(b.cfg.RobotCode),
		DownloadCode: downloadCode,
	})
	if err != nil {
		return "", fmt.Errorf("dingtalk: media download: %w", err)
	}
	if url := strings.TrimSpace(result.DownloadURL); url != "" {
		return url, nil
	}
	return "", errors.New("dingtalk: media download: empty downloadUrl")
}
