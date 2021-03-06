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
	"regexp"
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

// SHEETIDFPL FOR FPL
const SHEETIDFPL = "1hxjwncIybtnIrDUbd3_NL6_6MdvaDC1L58gBWdux9sY"

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
	// fmt.Printf("time:\t%v\nmesId:\t%v\nremoteId:\t%v\nquoteMessageId:\t%v\nsenderId:\t%v\nmessage:\t%v\n", message.Info.Timestamp, message.Info.Id, message.Info.RemoteJid, message.ContextInfo.QuotedMessageID, message.Info.SenderJid, message.Text)
	if !strings.Contains(strings.ToLower(message.Text), "#fpllifc#") || strings.Contains(strings.ToLower(message.Text), "<") || strings.Contains(strings.ToLower(message.Text), ">") || message.Info.Timestamp < wh.startTime {
		return
	}

	code := strings.Split(message.Text, "#")
	remoteID := strings.Split(message.Info.RemoteJid, "@")

	if len(code) == 5 && strings.EqualFold(code[0], "REG") && strings.EqualFold(code[1], "FPLLIFC") && !strings.Contains(remoteID[1], "g") && message.Info.Timestamp > wh.startTime {
		errorMessage := getErrorMessageV2(1002)
		sendMessageV2(message, errorMessage, wh.wac, nil)
		return
	}

	if len(code) == 5 && strings.EqualFold(code[0], "REG") && strings.EqualFold(code[1], "FPLLIFC") && message.Info.Timestamp > wh.startTime {
		isPhoneNumber := isPhoneNumber(code[2])

		var messageSent string
		if isPhoneNumber {
			messageSent = registerFPLV2(code)
		} else {
			messageSent = getErrorMessageV2(1003)
		}

		sendMessageV2(message, messageSent, wh.wac, nil)
	} else if len(code) == 3 && strings.EqualFold(code[0], "CHECK") && strings.EqualFold(code[1], "FPLLIFC") && message.Info.Timestamp > wh.startTime {
		isPhoneNumber := isPhoneNumber(code[2])

		var messageSent string
		if isPhoneNumber {
			messageSent = checkRegisterFPL(code[2])
			// fmt.Println(messageSent)
		} else {
			messageSent = getErrorMessageV2(1004)
		}

		if messageSent == "" {
			return
		}

		sendMessageV2(message, messageSent, wh.wac, nil)
	} else {
		return
	}
}

func getErrorMessageV2(code int) string {
	var msg string

	switch errorCode := code; errorCode {
	case 1001:
		msg = "Kesalahan Format Penulisan Pendaftaran. Mohon Ulangi."
	case 1002:
		msg = "Melakukan pendaftaran FPLLIFC2020/2021 hanya bisa dilakukan di grup, silahkan kunjungi grup https://s.id/fpllifc . \n\nSalam,\n\n\nLIFCia"
	case 1003:
		msg = "Nomor WA yang ingin didaftarkan tidak sesuai format\n\nPesan dikirim oleh LIFCia 😊"
	case 1004:
		msg = "Nomor WA yang ingin dicek tidak sesuai format \n\nPesan dikirim oleh LIFCia 😊"
	}

	return msg
}

func sendMessageV2(messageReceived whatsapp.TextMessage, messageSent string, wac *whatsapp.Conn, code []string) {
	var participant string
	remoteID := strings.Split(messageReceived.Info.RemoteJid, "@")

	if strings.Contains(remoteID[1], "g") && code != nil {
		participant = code[2] + "@s.whatsapp.net"
	} else {
		participant = messageReceived.Info.RemoteJid
	}

	previousMessage := messageReceived.Text
	quotedMessage := proto.Message{
		Conversation: &previousMessage,
	}

	ContextInfo := whatsapp.ContextInfo{
		QuotedMessage:   &quotedMessage,
		QuotedMessageID: messageReceived.Info.Id,
		Participant:     participant,
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

func checkRegisterFPL(phone string) string {
	var msg string
	isRegistered, row, err := isRegisteredInSheet(phone)
	if err != nil {
		fmt.Println(err)
		return msg
	}

	if isRegistered {
		msg = "Anda telah terdaftar, \n" +
			"\nOfficial LIFC Classic League Team : " + row[2].(string) +
			"\nOfficial LIFC H2H League Team : " + row[3].(string) +
			"\nKontak : " + row[1].(string) +
			"\n\n\nPesan dikirim oleh LIFCia 😊"
	} else if !isRegistered && err == nil {
		msg = "Manager dengan Nomor WA " + phone + " belum mendaftarkan tim nya, silahkan lakukan pendaftaran dengan format seperti yang ada di deskripsi grup ini. \n\nPesan dikirim oleh LIFCia 😊"
	}

	return msg
}

func isRegisteredInSheet(phone string) (bool, []interface{}, error) {
	var isRegistered = false
	var error error

	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		error = err
		fmt.Println(err)
		return isRegistered, nil, error
		// return false
	}

	config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets")
	if err != nil {
		error = err
		fmt.Println(err)
		return isRegistered, nil, error
		// return false
	}
	client := getClient(config)

	srv, err := sheets.New(client)
	if err != nil {
		error = err
		fmt.Println(err)
		return isRegistered, nil, error
		// return false
	}

	spreadsheetID := SHEETIDFPL
	i := 0
	readRange := "Sheet1!A1:D"
	resp, err := srv.Spreadsheets.Values.Get(spreadsheetID, readRange).Do()
	if err != nil {
		error = err
		fmt.Println(err)
		return isRegistered, nil, error
		// return false
	}

	var rowData []interface{}

	if len(resp.Values) == 0 {
		fmt.Println("No data found.")
	} else {
		for _, row := range resp.Values {
			// fmt.Println(row[1])
			if row[1] == phone {
				isRegistered = true
				rowData = row
				break
			} else {
				if row[0] == "" {
					rowData = nil
					isRegistered = false
					break
				} else {
					i++
				}
			}
		}
	}

	return isRegistered, rowData, nil
}

func isPhoneNumber(phone string) bool {
	isPhoneNumber := regexp.MustCompile(`^[+]*[0-9]{9,20}$`).MatchString
	return isPhoneNumber(phone)
}

func registerFPLV2(code []string) string {
	var msg string
	isSent := sendDataToSpreadSheet(code, time.Now().Format("01-02-2006 15:04:05"))

	if !isSent {
		msg = "Gagal melakukan pendaftaran\n\nOfficial LIFC Classic League Team: " +
			code[3] + ",\nOfficial LIFC H2H League Team: " +
			code[4] + ",\nKontak: " +
			code[2] + "\n\nPesan dikirim oleh LIFCia" +
			"\nTolong beritahu rekan LIFCia ya, terimakasih 😊"
	} else {
		msg = "Berhasil melakukan pendaftaran\n\nOfficial LIFC Classic League Team: " +
			code[3] + ",\nOfficial LIFC H2H League Team: " +
			code[4] + ",\nKontak: " +
			code[2] + "\n\nTerimakasih\nPesan dikirim oleh LIFCia 😊"
	}

	return msg
}

func sendDataToSpreadSheet(code []string, timestamp string) bool {
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		fmt.Println(err)
		return false
	}

	config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets")
	if err != nil {
		fmt.Println(err)
		return false
	}
	client := getClient(config)

	srv, err := sheets.New(client)
	if err != nil {
		fmt.Println(err)
		return false
	}

	spreadsheetID := SHEETIDFPL
	i := 0
	readRange := "Sheet1!A1:D"
	resp, err := srv.Spreadsheets.Values.Get(spreadsheetID, readRange).Do()
	if err != nil {
		fmt.Println(err)
		return false
	}

	if len(resp.Values) == 0 {
		fmt.Println("No data found.")
	} else {
		for _, row := range resp.Values {
			if row[0] == "" {
				i++
				// fmt.Printf("%d\n", i)
				break
			} else {
				if row[1] == code[2] {
					break
				} else {
					i++
				}
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
			return fmt.Errorf("restoring failed: %v", err)
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
			return fmt.Errorf("error during login: %v", err)
		}
	}

	//save session
	err = writeSession(session)
	if err != nil {
		return fmt.Errorf("error saving session: %v", err)
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
