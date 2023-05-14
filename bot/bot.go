package bot

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"bot_module/common"
	"bot_module/keyboard"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

/****************
* Types
****************/

type State uint8

type Item struct {
	common.JItem
	JDate common.Date
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
var jobs map[common.Date][]common.JItem
var subscribers map[int64]chan struct{}

/****************
* Init function
****************/
func init() {
	CurrState = Idle
	jobs = make(map[common.Date][]common.JItem)
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

func SubscribeCmdResponse(tgbot *tgbotapi.BotAPI, chatID int64) string {
	var msg string
	if CurrState == Idle {
		if _, ok := subscribers[chatID]; ok {
			msg = "You are already subscribed. To unsubscribe enter /unsubscribe command."
		} else {
			msg = "You are subscribed now. To unsubscribe enter /unsubscribe command."
			ch := make(chan struct{})
			subscribers[chatID] = ch
			go remindJobs(tgbot, chatID, period, ch)
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

func OnMsgDateWaitAddResponse(item *Item, answer string) (string, error) {
	date, err := time.Parse(dateFormat, answer)
	if err != nil {
		return "Could not parse date. Try again in format dd-mm-yyyy.", err
	}
	item.JDate = timeToDate(date)
	jobs[item.JDate] = append(jobs[item.JDate], item.JItem)
	CurrState = Idle
	return "Your job was added.", nil
}

func OnMsgDateWaitShowResponse(item *Item, answer string) (string, error) {
	date, err := time.Parse(dateFormat, answer)
	if err != nil {
		return "Could not parse date. Try again in format dd-mm-yyyy.", err
	}
	dateStruct := timeToDate(date)
	CurrState = Idle
	return getAllJobs(dateStruct), nil
}

func OnMsgDateWaitDoneResponse(answer string) (string, error) {
	date, err := time.Parse(dateFormat, answer)
	if err != nil {
		return "Could not parse date. Try again in format dd-mm-yyyy.", err
	}
	dateStruct := timeToDate(date)
	msg, ok := keyboard.ShowKeyboard(dateStruct, jobs[dateStruct])
	if ok {
		CurrState = WaitKeyPress
	} else {
		CurrState = Idle
	}
	return msg, nil
}

func OnMsgWaitKeyPressResponse(answer string) string {
	num, err := strconv.Atoi(answer)
	if err != nil || num < 0 || num > keyboard.ItemsCount() {
		return "Press the button on keyboard or type appropriate number."
	}
	keyboard.Hide()
	CurrState = Idle
	if num == 0 {
		return "Operation canceled."
	} else {
		num--
		date := keyboard.Date()
		itemInd := keyboard.Index(num)
		jobs[date][itemInd].Done = true
		return "Well done!"
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
			jobList := getAllJobs(date)
			msg := tgbotapi.NewMessage(subscriberID, jobList)
			if err := SendMsg(tgbot, &msg); err != nil {
				log.Printf("Something went wrong: %v\n", err)
			}
		case <-done:
			t.Stop()
			return
		}
	}
}

func getAllJobs(t common.Date) string {
	var msg string
	if len(jobs[t]) == 0 {
		msg = "No jobs were planned on this date."
	} else {
		for i, job := range jobs[t] {
			jobState := getDoneStr(job.Done)
			msg += fmt.Sprintf("%d. %s - %s\n", i+1, job.Job, jobState)
		}
	}
	return msg
}

func timeToDate(t time.Time) common.Date {
	y, m, d := t.Date()
	return common.Date{y, m, d}
}

func getDoneStr(b bool) string {
	if b {
		return "Done"
	} else {
		return "TODO"
	}
}
