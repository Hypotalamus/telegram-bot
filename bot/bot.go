package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"bot_module/common"
	"bot_module/keyboard"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	bolt "go.etcd.io/bbolt"
)

/****************
* Types
****************/

type State uint8

type Item struct {
	common.JItem
	JDate common.Date
}

type JKey struct {
	ChatID int64
	Date   common.Date
}

/****************
* Constants
****************/

const dateFormat = "02-01-2006"
const period = 10 * time.Second

const (
	Idle State = iota
	JobWait
	DateWaitAdd
	DateWaitShow
	DateWaitDone
	WaitKeyPress
)

/****************
* Variables
****************/

var CurrState State
var muSendMsg sync.Mutex
var subscribers map[int64]chan struct{}

/****************
* Init function
****************/
func init() {
	CurrState = Idle
	subscribers = make(map[int64]chan struct{})
}

/*****************************************************************
* User's message handlers
*****************************************************************/

func StartCmdResponse() string {
	if CurrState == Idle {
		return "Hello. I am TODO bot. You can add to me your jobs with " +
			"dates when it should be done and I remind it to.\n" +
			"Supported commands: \n" +
			"/start - show this message\n" +
			"/newitem - add new job\n" +
			"/items - show all jobs for some day\n" +
			"/subscribe - subscribe to job reminder\n" +
			"/unsubscribe - unsubscribe from job reminder\n" +
			"/done - mark some TODO job done\n" +
			"Have a good day!"
	} else {
		return "Please complete current operation or cancel it using /cancel."
	}
}

func NewItemCmdResponse() string {
	if CurrState == Idle {
		CurrState = JobWait
		return "Please Enter new job."
	} else {
		return "Please complete current operation or cancel it using /cancel."
	}
}

func ItemsCmdResponse() string {
	if CurrState == Idle {
		CurrState = DateWaitShow
		return "Please enter date for which you want to see jobs in format dd-mm-yyyy."
	} else {
		return "Please complete current operation or cancel it using /cancel."
	}
}

func CancelCmdResponse() string {
	if CurrState != Idle {
		if keyboard.IsVisible() {
			keyboard.Hide()
		}
		CurrState = Idle
		return "Current operation was cancelled."
	} else {
		return "I am Idle already."
	}
}

func SubscribeCmdResponse(tgbot *tgbotapi.BotAPI, chatID int64, db *bolt.DB) string {
	var msg string
	if CurrState == Idle {
		if _, ok := subscribers[chatID]; ok {
			msg = "You are already subscribed. To unsubscribe enter /unsubscribe command."
		} else {
			msg = "You are subscribed now. To unsubscribe enter /unsubscribe command."
			ch := make(chan struct{})
			subscribers[chatID] = ch
			go remindJobs(tgbot, chatID, period, ch, chatID, db)
		}
	} else {
		msg = "Please complete current operation or cancel it using /cancel."
	}
	return msg
}

func UnsubscribeCmdResponse(chatID int64) string {
	var msg string
	if CurrState == Idle {
		if _, ok := subscribers[chatID]; !ok {
			msg = "You are not subscribed."
		} else {
			msg = "You are unsubscribed now."
			close(subscribers[chatID])
			delete(subscribers, chatID)
		}
	} else {
		msg = "Please complete current operation or cancel it using /cancel."
	}
	return msg
}

func DoneCmdResponse() string {
	if CurrState == Idle {
		CurrState = DateWaitDone
		return "Please enter day where done job is placed in format dd-mm-yyyy."
	} else {
		return "Please complete current operation or cancel it using /cancel."
	}
}

func UnknownCmdResponse() string {
	return "I don't know that command. Enter /start to see list of commands."
}

func OnMsgIdleResponse() string {
	return "Please enter some command. Enter /start to see list of commands."
}

func OnMsgJobWaitResponse(item *Item, answer string) string {
	item.JItem.Job = answer
	CurrState = DateWaitAdd
	return "Please Enter date in format dd-mm-yyyy."
}

func OnMsgDateWaitAddResponse(
	item *Item,
	answer string,
	chatID int64,
	db *bolt.DB,
) (string, error) {
	date, err := time.Parse(dateFormat, answer)
	if err != nil {
		return "Could not parse date. Try again in format dd-mm-yyyy.", err
	}
	dateStruct := timeToDate(date)
	item.JDate = dateStruct
	CurrState = Idle
	jobs, err := getData(db, chatID, dateStruct)
	if err != nil {
		return "Could not read from database. Try operation again.", err
	}
	jobs = append(jobs, item.JItem)
	err = putData(db, chatID, dateStruct, jobs)
	if err != nil {
		return "Could not write to database. Try operation again.", err
	}
	return "Your job was added.", nil
}

func OnMsgDateWaitShowResponse(
	item *Item,
	answer string,
	chatID int64,
	db *bolt.DB,
) (string, error) {
	date, err := time.Parse(dateFormat, answer)
	if err != nil {
		return "Could not parse date. Try again in format dd-mm-yyyy.", err
	}
	dateStruct := timeToDate(date)
	joblist, err := getAllJobs(dateStruct, chatID, db)
	CurrState = Idle
	if err != nil {
		return "Could not read from database. Try operation again.", err
	}
	return joblist, nil
}

func OnMsgDateWaitDoneResponse(answer string, chatID int64, db *bolt.DB) (string, error) {
	date, err := time.Parse(dateFormat, answer)
	if err != nil {
		return "Could not parse date. Try again in format dd-mm-yyyy.", err
	}
	dateStruct := timeToDate(date)
	jobs, err := getData(db, chatID, dateStruct)
	if err != nil {
		return "Could not read from database. Try operation again", err
	}
	msg, ok := keyboard.ShowKeyboard(dateStruct, jobs)
	if ok {
		CurrState = WaitKeyPress
	} else {
		CurrState = Idle
	}
	return msg, nil
}

func OnMsgWaitKeyPressResponse(answer string, chatID int64, db *bolt.DB) (string, error) {
	num, err := strconv.Atoi(answer)
	if err != nil || num < 0 || num > keyboard.ItemsCount() {
		return "Press the button on keyboard or type appropriate number.", nil
	}
	keyboard.Hide()
	CurrState = Idle
	if num == 0 {
		return "Operation canceled.", nil
	} else {
		num--
		date := keyboard.Date()
		itemInd := keyboard.Index(num)
		jobs, err := getData(db, chatID, date)
		if err != nil {
			return "Could not read from database. Try operation again.", err
		}
		jobs[itemInd].Done = true
		err = putData(db, chatID, date, jobs)
		if err != nil {
			return "Could not write to database. Try operation again.", err
		}
		return "Well done!", nil
	}

}

/*****************************************************************
* Utility functions
*****************************************************************/

func SendMsg(tgbot *tgbotapi.BotAPI, msg *tgbotapi.MessageConfig) error {
	muSendMsg.Lock()
	_, err := tgbot.Send(msg)
	muSendMsg.Unlock()
	return err
}

func remindJobs(
	tgbot *tgbotapi.BotAPI,
	subscriberID int64,
	period time.Duration,
	done chan struct{},
	chatID int64,
	db *bolt.DB,
) {
	tNow := time.Now()
	// yyyy, mm, dd := tNow.Date()
	nextStart := tNow.Add(1 * time.Minute)
	// nextStart := time.Date(yyyy, mm, dd+1, 9, 0, 0, 0, tNow.Location())
	diff := nextStart.Sub(tNow)
	preTimer := time.NewTimer(diff)
	select {
	case <-preTimer.C:
	case <-done:
		preTimer.Stop()
		return
	}
	t := time.NewTicker(period)
	for {
		select {
		case now := <-t.C:
			date := timeToDate(now)
			joblist, err := getAllJobs(date, chatID, db)
			if err != nil {
				log.Printf("Could not read from database: %v\n", err)
			}
			msg := tgbotapi.NewMessage(subscriberID, joblist)
			if err = SendMsg(tgbot, &msg); err != nil {
				log.Printf("Could not send message to Telegram: %v\n", err)
			}
		case <-done:
			t.Stop()
			return
		}
	}
}

func getAllJobs(t common.Date, chatID int64, db *bolt.DB) (string, error) {
	jobs, err := getData(db, chatID, t)
	if err != nil {
		return "Could not read from database.", err
	}
	msg := ""
	if len(jobs) == 0 {
		msg = "No jobs were planned on this date."
	} else {
		for i, job := range jobs {
			jobState := getDoneStr(job.Done)
			msg += fmt.Sprintf("%d. %s - %s\n", i+1, job.Job, jobState)
		}
	}
	return msg, nil
}

func timeToDate(t time.Time) common.Date {
	y, m, d := t.Date()
	return common.Date{Year: y, Month: m, Day: d}
}

func getDoneStr(b bool) string {
	if b {
		return "Done"
	} else {
		return "TODO"
	}
}

/*****************************
* Work with data storage
*****************************/
func putData(db *bolt.DB, chatID int64, date common.Date, data []common.JItem) error {
	err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Default"))
		key := JKey{ChatID: chatID, Date: date}
		keyBytes, err := json.Marshal(key)
		if err != nil {
			return err
		}
		valBytes, err := json.Marshal(data)
		if err != nil {
			return err
		}
		err = b.Put(keyBytes, valBytes)
		return err
	})
	return err
}

func getData(db *bolt.DB, chatID int64, date common.Date) ([]common.JItem, error) {
	res := []common.JItem{}
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Default"))
		key := JKey{ChatID: chatID, Date: date}
		keyBytes, err := json.Marshal(key)
		if err != nil {
			return err
		}
		v := b.Get(keyBytes)
		if v != nil {
			err = json.Unmarshal(v, &res)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return res, err
}

func SetupDB(db *bolt.DB) error {
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("Default"))
		return err
	})
	return err
}
