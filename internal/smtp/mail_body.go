package smtp

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/emersion/go-message"
	gomail "github.com/emersion/go-message/mail"
	htmlcharset "golang.org/x/net/html/charset"
)

type mimeBody struct {
	text        string
	html        string
	attachments []Attachment
}

// MIME structure matters: alternative parts replace representations, mixed
// parts extend the body, and related parts have one root plus resources.
// Flattening all three lets an inline image or another text part erase mail.
func readMIMEBody(entity *message.Entity, parts *int, resource bool) (mimeBody, error) {
	(*parts)++
	if *parts > 100 {
		return mimeBody{}, fmt.Errorf("mail exceeds 100 MIME parts")
	}
	mediaType, params, _ := entity.Header.ContentType()
	disposition, _, _ := entity.Header.ContentDisposition()
	attachmentHeader := gomail.AttachmentHeader{Header: entity.Header}
	filename, _ := attachmentHeader.Filename()
	attachment := resource || disposition == "attachment" || filename != ""

	if !attachment && strings.HasPrefix(mediaType, "multipart/") {
		reader := entity.MultipartReader()
		defer reader.Close()
		var result mimeBody
		for index := 0; ; index++ {
			child, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				if child == nil || (!message.IsUnknownCharset(err) && !message.IsUnknownEncoding(err)) {
					return mimeBody{}, fmt.Errorf("read MIME part: %w", err)
				}
				logMailError("reading MIME part with unsupported encoding: %v", err)
			}
			isResource := false
			if mediaType == "multipart/related" {
				if start := params["start"]; start != "" {
					isResource = strings.Trim(child.Header.Get("Content-ID"), "<> ") != strings.Trim(start, "<> ")
				} else {
					isResource = index != 0
				}
			}
			body, err := readMIMEBody(child, parts, isResource)
			if err != nil {
				return mimeBody{}, err
			}
			result.attachments = append(result.attachments, body.attachments...)
			if mediaType == "multipart/alternative" {
				// RFC 2046: later supported alternatives are preferred. Retain
				// both the plain and HTML representations for their consumers.
				if body.text != "" {
					result.text = body.text
				}
				if body.html != "" {
					result.html = body.html
				}
			} else {
				if result.html != "" || body.html != "" {
					result.html = joinMIMEText(result.asHTML(), body.asHTML())
				}
				result.text = joinMIMEText(result.text, body.text)
			}
		}
		return result, nil
	}

	body, err := io.ReadAll(entity.Body)
	if err != nil {
		return mimeBody{}, fmt.Errorf("read %s body: %w", mediaType, err)
	}
	if !attachment {
		switch mediaType {
		case "text/plain":
			return mimeBody{text: string(body)}, nil
		case "text/html":
			// go-message already decodes MIME charsets. Only consult HTML's
			// charset metadata if bytes are still not UTF-8, avoiding a second
			// conversion of a decoded body with an unchanged <meta charset>.
			if !utf8.Valid(body) {
				reader, err := htmlcharset.NewReader(bytes.NewReader(body), "")
				if err != nil {
					return mimeBody{}, fmt.Errorf("detect HTML charset: %w", err)
				}
				body, err = io.ReadAll(reader)
				if err != nil {
					return mimeBody{}, fmt.Errorf("decode HTML charset: %w", err)
				}
			}
			return mimeBody{html: string(body)}, nil
		}
	}
	contentType := entity.Header.Get("Content-Type")
	if contentType == "" {
		contentType = mediaType
	}
	if filename == "" {
		// Give non-body inline resources useful document names as well.
		filename = strings.Trim(entity.Header.Get("Content-ID"), "<> ")
		if filename == "" {
			filename = "attachment"
		}
	}
	inlineResource := disposition != "attachment" && (resource || (disposition == "inline" && strings.HasPrefix(mediaType, "image/")))
	return mimeBody{attachments: []Attachment{{
		Filename: filename, ContentType: contentType, Content: body, InlineResource: inlineResource,
	}}}, nil
}

func joinMIMEText(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return strings.TrimRight(a, "\r\n") + "\n\n" + strings.TrimLeft(b, "\r\n")
}

func (b mimeBody) asHTML() string {
	if b.html != "" {
		return b.html
	}
	if b.text == "" {
		return ""
	}
	text := strings.ReplaceAll(b.text, "\r\n", "\n")
	return "<div>" + strings.ReplaceAll(html.EscapeString(text), "\n", "<br>") + "</div>"
}
