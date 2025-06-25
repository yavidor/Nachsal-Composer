package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var timestamp time.Time

type soldierJson struct {
	Name   string `json:"name"`
	Number string `json:"number"`
}
type Soldier struct {
	name    string
	jid     types.JID
	message string
}

func composeMessage(soldiers []*Soldier) string {
	output := ""
	for index, soldier := range soldiers {
		output += fmt.Sprintf("*%s*\n%s\n", soldier.name, "תקין")
		if index != len(soldiers)-1 {
			output += "--------------------\n"
		}
	}
	return output
}

func getSoldierAuthor(msg *events.Message, soldiers []*Soldier) *Soldier {
	for _, soldier := range soldiers {
		if soldier.jid.User == msg.Info.Sender.User {
			return soldier
		}
	}
	return nil
}

func allSoldiersAnswered(soldiers []*Soldier) bool {
	for _, soldier := range soldiers {
		if soldier.message == "" {
			return false
		}
	}
	return true
}

func reactWithLike(soldiers []*Soldier) func(*WhatsappService, *events.Message) error {
	return func(s *WhatsappService, msg *events.Message) error {
		if soldier := getSoldierAuthor(msg, soldiers); soldier != nil && soldier.message == "" && msg.Info.Timestamp.After(timestamp) {
			err := s.React(soldier.jid, msg.Info.Chat, msg.Info.ID, "👍")
			return err
		}
		return nil
	}
}

func registerMessage(soldiers []*Soldier) func(*WhatsappService, *events.Message) error {
	return func(s *WhatsappService, msg *events.Message) error {
		if soldier := getSoldierAuthor(msg, soldiers); soldier != nil && soldier.message == "" && msg.Info.Timestamp.After(timestamp) {
			var content string
			if msg.Message.ExtendedTextMessage == nil {
				content = msg.Message.GetConversation()
			} else {
				content = *msg.Message.GetExtendedTextMessage().Text
			}
			soldier.message = content
			fmt.Printf("%+v\n", soldier)
		}
		return nil
	}
}

const BEGINNING_MESSAGE = "היי, הכל בסדר?"

func sendBeginningMessage(s *WhatsappService, soldiers []*Soldier) error {
	for _, soldier := range soldiers {
		err := s.SendMessage(BEGINNING_MESSAGE, soldier.jid.User, false)
		if err != nil {
			return err
		}
	}
	timestamp = time.Now()
	return nil
}

func sendIfFinished(soldiers []*Soldier, commanderNumber string) func(*WhatsappService, *events.Message) error {
	return func(s *WhatsappService, msg *events.Message) error {
		if allSoldiersAnswered(soldiers) {
			s.SendMessage(composeMessage(soldiers), commanderNumber, false)
			os.Exit(0)
		}
		return nil
	}
}

func printMessage(_ *WhatsappService, msg *events.Message) error {
	fmt.Printf("--------------------\n%+v\n--------------------\n", msg)
	fmt.Println(msg.Message.ExtendedTextMessage)
	return nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		panic("Error loading .env file")
	}
	COMMANDER_NUMBER := os.Getenv("COMMANDER_NUMBER")
	var teamB []*Soldier
	soldiersFile, err := os.ReadFile("soldiers.json")
	if err != nil {
		panic(err)
	}
	var soldiersJson []soldierJson
	if err = json.Unmarshal(soldiersFile, &soldiersJson); err != nil {
		panic(err)
	}
	for _, v := range soldiersJson {
		teamB = append(teamB, &Soldier{
			name:    v.Name,
			jid:     types.NewJID(v.Number, WHATSAPP_SERVER),
			message: "",
		})
	}
	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New("sqlite3", "file:examplestore.db?_foreign_keys=on&_journal_mode=WAL", dbLog)
	if err != nil {
		panic(err)
	}
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}
	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	fmt.Println("I'm tired")
	whatsapp := NewWhatsappService(client)
	whatsapp.
		OnMessage(reactWithLike(teamB)).
		OnMessage(registerMessage(teamB)).
		OnMessage(sendIfFinished(teamB, COMMANDER_NUMBER)).
		OnMessage(printMessage).
		Init()

	sendBeginningMessage(whatsapp, teamB)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	client.Disconnect()
}
