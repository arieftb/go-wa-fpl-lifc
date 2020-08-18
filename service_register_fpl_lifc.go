package main

import (
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"

	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Rhymen/go-whatsapp/binary/proto"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/sheets/v4"

	qrcodeTerminal "github.com/Baozisoftware/qrcode-terminal-go"
	whatsapp "github.com/Rhymen/go-whatsapp"
)

type waHandler struct {
	wac       *whatsapp.Conn
	startTime uint64
}

func (wh *waHandler) HandleError(err error) {

	if e, ok := err.(*whatsapp.ErrConnectionFailed); ok {
		log.Printf("Connection failed, underlying error: %v", e.Err)
		log.Println("Waiting 30sec...")
		<-time.After(30 * time.Second)
		log.Println("Reconnecting...")
		err := wh.wac.Restore()
		if err != nil {
			log.Fatalf("Restore failed: %v", err)
		}
	} else {
		log.Printf("error occoured: %v\n", err)
	}
}

func (wh *waHandler) HandleTextMessage(message whatsapp.TextMessage) {
	// var sendMessages whatsapp.TextMessage
	fmt.Printf("time:\t%v\nmesId:\t%v\nremoteId:\t%v\nquoteMessageId:\t%v\nsenderId:\t%v\nmessage:\t%v\n", message.Info.Timestamp, message.Info.Id, message.Info.RemoteJid, message.ContextInfo.QuotedMessageID, message.Info.SenderJid, message.Text)
	if !strings.Contains(strings.ToLower(message.Text), "#fpllifc#") || strings.Contains(strings.ToLower(message.Text), "<") || strings.Contains(strings.ToLower(message.Text), ">") || message.Info.Timestamp < wh.startTime {
		return
	}

	code := strings.Split(message.Text, "#")

	if len(code) == 5 && strings.EqualFold(code[0], "REG") && strings.EqualFold(code[1], "FPLLIFC") {
		messageSent := registerFPLV2(code)
		sendMessageV2(message, messageSent, wh.wac)
	} else {
		return
	}
}

func getErrorMessage(code int, message whatsapp.TextMessage) whatsapp.TextMessage {
	var mes string

	switch errorCode := code; errorCode {
	case 1001:
		mes = "Kesalahan Format Penulisan Pendaftaran. Mohon Ulangi."
	}

	previousMessage := message.Text
	quotedMessage := proto.Message{
		Conversation: &previousMessage,
	}

	ContextInfo := whatsapp.ContextInfo{
		QuotedMessage:   &quotedMessage,
		QuotedMessageID: message.Info.Id,
		// MentionedJid: "081510837507@s.whatsapp.net",
		Participant: "6281510837507@s.whatsapp.net",
	}

	var msg whatsapp.TextMessage

	msg = whatsapp.TextMessage{
		Info: whatsapp.MessageInfo{
			RemoteJid: "6281510837507@s.whatsapp.net",
		},
		ContextInfo: ContextInfo,
		Text:        mes + "\n\nThis message sent by LIFCia @081510837507",
	}

	return msg
}

func sendMessageV2(messageReceived whatsapp.TextMessage, messageSent string, wac *whatsapp.Conn) {
	previousMessage := messageReceived.Text
	quotedMessage := proto.Message{
		Conversation: &previousMessage,
	}

	ContextInfo := whatsapp.ContextInfo{
		QuotedMessage:   &quotedMessage,
		QuotedMessageID: messageReceived.Info.Id,
		Participant:     messageReceived.Info.RemoteJid,
	}

	msg := whatsapp.TextMessage{
		Info: whatsapp.MessageInfo{
			RemoteJid: messageReceived.Info.RemoteJid,
		},
		ContextInfo: ContextInfo,
		Text:        messageSent,
	}

	if _, err := wac.Send(msg); err != nil {
		fmt.Fprintf(os.Stderr, "error sending message: %v\n", err)
	}
}

func registerFPLV2(code []string) string {
	var msg string
	isSent := sendDataToSpreadSheet(code, time.Now().Format("01-02-2006 15:04:05"))

	if !isSent {
		msg = "UNSUCCESSFUL REGISTERED\nOfficial LIFC Classic League Team: " +
			code[3] + ",\nOfficial LIFC H2H League Team: " +
			code[4] + ",\nFrom: " +
			code[2] + "\n\nThis message sent by LIFCia" +
			"\nPlease contact the owner of LIFCia"
	} else {
		msg = "Registered\nOfficial LIFC Classic League Team: " +
			code[3] + ",\nOfficial LIFC H2H League Team: " +
			code[4] + ",\nFrom: " +
			code[2] + "\n\nThis message sent by LIFCia"
	}

	return msg
}

func sendDataToSpreadSheet(code []string, timestamp string) bool {
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		println(err)
		return false
	}

	config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets")
	if err != nil {
		println(err)
		return false
	}
	client := getClient(config)

	srv, err := sheets.New(client)
	if err != nil {
		println(err)
		return false
	}

	spreadsheetID := "1hxjwncIybtnIrDUbd3_NL6_6MdvaDC1L58gBWdux9sY"
	i := 0
	readRange := "Sheet1!A1:A"
	resp, err := srv.Spreadsheets.Values.Get(spreadsheetID, readRange).Do()
	if err != nil {
		println(err)
		return false
	}

	if len(resp.Values) == 0 {
		fmt.Println("No data found.")
	} else {
		for _, row := range resp.Values {
			if row[0] == "" {
				// fmt.Printf("%d\n", i)
				break
			} else {
				i++
				// fmt.Printf("%d\n", i)
				// fmt.Printf("%s\n", row[0])
			}
		}
	}

	rangeInputData := "Sheet1!A" + strconv.Itoa(i+1) + ":D" + strconv.Itoa(i+1)
	values := [][]interface{}{{timestamp, "'" + code[2], code[3], code[4]}}

	rb := &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "USER_ENTERED",
	}
	rb.Data = append(rb.Data, &sheets.ValueRange{
		Range:  rangeInputData,
		Values: values,
	})
	_, err = srv.Spreadsheets.Values.BatchUpdate(spreadsheetID, rb).Context(context.Background()).Do()
	if err != nil {
		println(err)
		return false
	}
	fmt.Println("Done.")
	return true
}

func getClient(config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func checkError(err error) {
	if err != nil {
		panic(err.Error())
	}
}

func main() {
	//create new WhatsApp connection
	wac, err := whatsapp.NewConn(5 * time.Second)
	if err != nil {
		log.Fatalf("error creating connection: %v\n", err)
	}

	//Add handler
	wac.AddHandler(&waHandler{wac, uint64(time.Now().Unix())})

	// wac.SetClientVersion(2, 2021, 4)
	//login or restore
	if err := login(wac); err != nil {
		log.Fatalf("error logging in: %v\n", err)
	}

	//verifies phone connectivity
	pong, err := wac.AdminTest()

	if !pong || err != nil {
		log.Fatalf("error pinging in: %v\n", err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	//Disconnect safe
	fmt.Println("Shutting down now.")
	session, err := wac.Disconnect()
	if err != nil {
		log.Fatalf("error disconnecting: %v\n", err)
	}
	if err := writeSession(session); err != nil {
		log.Fatalf("error saving session: %v", err)
	}
}

func login(wac *whatsapp.Conn) error {
	//load saved session
	session, err := readSession()
	if err == nil {
		//restore session
		session, err = wac.RestoreWithSession(session)
		if err != nil {
			return fmt.Errorf("restoring failed: %v\n", err)
		}
	} else {
		//no saved session -> regular login
		qr := make(chan string)
		go func() {
			terminal := qrcodeTerminal.New()
			terminal.Get(<-qr).Print()
			println(qr)
			// terminal.Get("HALLO").Print()
		}()
		session, err = wac.Login(qr)
		if err != nil {
			return fmt.Errorf("error during login: %v\n", err)
		}
	}

	//save session
	err = writeSession(session)
	if err != nil {
		return fmt.Errorf("error saving session: %v\n", err)
	}
	return nil
}

func readSession() (whatsapp.Session, error) {
	session := whatsapp.Session{}
	file, err := os.Open(os.TempDir() + "/whatsappSession.gob")
	if err != nil {
		return session, err
	}
	defer file.Close()
	decoder := gob.NewDecoder(file)
	err = decoder.Decode(&session)
	if err != nil {
		return session, err
	}
	return session, nil
}

func writeSession(session whatsapp.Session) error {
	file, err := os.Create(os.TempDir() + "/whatsappSession.gob")
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := gob.NewEncoder(file)
	err = encoder.Encode(session)
	if err != nil {
		return err
	}
	return nil
}
