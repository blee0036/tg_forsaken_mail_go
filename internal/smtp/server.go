package smtp

import (
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/emersion/go-message"
	gomail "github.com/emersion/go-message/mail"
	gosmtp "github.com/emersion/go-smtp"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
)

// charsetMap maps lowercase charset names to their encoding.
var charsetMap = map[string]encoding.Encoding{
	"gb2312":      simplifiedchinese.GBK, // GBK is a superset of GB2312
	"gbk":         simplifiedchinese.GBK,
	"gb18030":     simplifiedchinese.GB18030,
	"hz-gb-2312":  simplifiedchinese.HZGB2312,
	"big5":        traditionalchinese.Big5,
	"euc-kr":      korean.EUCKR,
	"euc-jp":      japanese.EUCJP,
	"iso-2022-jp": japanese.ISO2022JP,
	"shift_jis":   japanese.ShiftJIS,
	"windows-874": charmap.Windows874,
	"koi8-r":      charmap.KOI8R,
	"koi8-u":      charmap.KOI8U,
	"utf-16be":    unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM),
	"utf-16le":    unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM),
}

func init() {
	// Also add windows-125x and iso-8859-x from charmap
	for i := 0; i <= 8; i++ {
		name := fmt.Sprintf("windows-125%d", i)
		switch i {
		case 0:
			charsetMap[name] = charmap.Windows1250
		case 1:
			charsetMap[name] = charmap.Windows1251
		case 2:
			charsetMap[name] = charmap.Windows1252
		case 3:
			charsetMap[name] = charmap.Windows1253
		case 4:
			charsetMap[name] = charmap.Windows1254
		case 5:
			charsetMap[name] = charmap.Windows1255
		case 6:
			charsetMap[name] = charmap.Windows1256
		case 7:
			charsetMap[name] = charmap.Windows1257
		case 8:
			charsetMap[name] = charmap.Windows1258
		}
	}

	// Register charset decoder for go-message
	message.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		cs := strings.ToLower(strings.TrimSpace(charset))

		// UTF-8 needs no transform
		if cs == "utf-8" || cs == "us-ascii" || cs == "ascii" || cs == "" {
			return input, nil
		}

		if enc, ok := charsetMap[cs]; ok {
			return enc.NewDecoder().Reader(input), nil
		}

		// Fallback: return input as-is with a warning rather than failing
		log.Printf("warning: unsupported charset %q, reading as raw bytes", charset)
		return input, nil
	}
}

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
		parsed.From = header.Get("From")
	}

	if toList, err := header.AddressList("To"); err == nil && len(toList) > 0 {
		parsed.To = toList[0].String()
	} else {
		parsed.To = header.Get("To")
	}

	if subject, err := header.Subject(); err == nil {
		parsed.Subject = subject
	} else {
		parsed.Subject = header.Get("Subject")
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
