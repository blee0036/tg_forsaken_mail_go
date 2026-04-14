package smtp

import (
	"fmt"
	"io"
	"log"
	"mime"
	"strings"

	_ "github.com/emersion/go-message/charset" // Register extended charset decoders (gb2312, gbk, big5, etc.)
	gomail "github.com/emersion/go-message/mail"
	gosmtp "github.com/emersion/go-smtp"
	"golang.org/x/text/encoding/ianaindex"
)

// Attachment represents an email attachment.
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// ParsedMail represents a parsed email message.
type ParsedMail struct {
	From        string
	To          string
	Subject     string
	Date        string
	Text        string
	HTML        string
	Attachments []Attachment
}

// MailHandler is a callback type for handling incoming mail.
type MailHandler func(mail *ParsedMail)

// Server is an SMTP server that receives mail and invokes a handler.
type Server struct {
	host    string
	port    int
	handler MailHandler
}

// New creates a new SMTP server instance.
func New(host string, port int) *Server {
	return &Server{
		host: host,
		port: port,
	}
}

// OnMessage registers a callback for when mail arrives.
func (s *Server) OnMessage(handler MailHandler) {
	s.handler = handler
}

// Start starts the SMTP server (blocking).
func (s *Server) Start() error {
	be := &backend{handler: s.handler}
	srv := gosmtp.NewServer(be)

	srv.Addr = fmt.Sprintf("%s:%d", s.host, s.port)
	srv.Domain = "localhost"
	srv.AllowInsecureAuth = true

	log.Printf("SMTP server listening on %s", srv.Addr)
	return srv.ListenAndServe()
}

// decodeRFC2047 decodes RFC 2047 encoded-words (e.g. =?utf-8?q?...?=) in a header value.
// Returns the original string if decoding fails.
var mimeDecoder = &mime.WordDecoder{
	CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
		// Reuse go-message's registered charset decoders
		// The _ import of go-message/charset registers them globally,
		// but mime.WordDecoder needs its own CharsetReader.
		// We delegate to golang.org/x/text via ianaindex.
		enc, err := ianaindex.MIME.Encoding(charset)
		if err != nil {
			return input, nil // fallback: return raw bytes
		}
		if enc == nil {
			return input, nil // UTF-8, no transform needed
		}
		return enc.NewDecoder().Reader(input), nil
	},
}

func decodeRFC2047(s string) string {
	decoded, err := mimeDecoder.DecodeHeader(s)
	if err != nil {
		return s
	}
	return decoded
}

// ParseMail parses a MIME message from a reader and returns a ParsedMail struct.
// This function can be used independently for testing.
func ParseMail(r io.Reader) (*ParsedMail, error) {
	mr, err := gomail.CreateReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create mail reader: %w", err)
	}
	defer mr.Close()

	parsed := &ParsedMail{}

	// Extract headers
	header := mr.Header

	if fromList, err := header.AddressList("From"); err == nil && len(fromList) > 0 {
		parsed.From = fromList[0].String()
	} else {
		parsed.From = decodeRFC2047(header.Get("From"))
	}

	if toList, err := header.AddressList("To"); err == nil && len(toList) > 0 {
		parsed.To = toList[0].String()
	} else {
		parsed.To = decodeRFC2047(header.Get("To"))
	}

	if subject, err := header.Subject(); err == nil {
		parsed.Subject = subject
	} else {
		parsed.Subject = decodeRFC2047(header.Get("Subject"))
	}

	if date, err := header.Date(); err == nil {
		if !date.IsZero() {
			parsed.Date = date.String()
		}
	} else {
		parsed.Date = header.Get("Date")
	}

	// Parse body parts and attachments
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("error reading mail part: %v", err)
			continue
		}

		switch h := part.Header.(type) {
		case *gomail.InlineHeader:
			contentType := h.Get("Content-Type")
			body, err := io.ReadAll(part.Body)
			if err != nil {
				log.Printf("error reading inline part: %v", err)
				continue
			}
			if strings.HasPrefix(contentType, "text/html") {
				parsed.HTML = string(body)
			} else {
				// text/plain or default
				parsed.Text = string(body)
			}
		case *gomail.AttachmentHeader:
			filename, _ := h.Filename()
			contentType := h.Get("Content-Type")
			body, err := io.ReadAll(part.Body)
			if err != nil {
				log.Printf("error reading attachment: %v", err)
				continue
			}
			parsed.Attachments = append(parsed.Attachments, Attachment{
				Filename:    filename,
				ContentType: contentType,
				Content:     body,
			})
		}
	}

	return parsed, nil
}

// backend implements the gosmtp.Backend interface.
type backend struct {
	handler MailHandler
}

func (b *backend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	return &session{handler: b.handler}, nil
}

// session implements the gosmtp.Session interface.
type session struct {
	handler MailHandler
	from    string
	to      []string
}

func (s *session) Mail(from string, opts *gosmtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, opts *gosmtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	parsed, err := ParseMail(r)
	if err != nil {
		log.Printf("error parsing mail: %v", err)
		return err
	}

	// If headers were not set from MIME, use envelope data
	if parsed.From == "" {
		parsed.From = s.from
	}
	if parsed.To == "" && len(s.to) > 0 {
		parsed.To = s.to[0]
	}

	if s.handler != nil {
		s.handler(parsed)
	}
	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.to = nil
}

func (s *session) Logout() error {
	return nil
}
