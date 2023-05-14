/* TODO:
- what if user type /cancel when keyboard is on screen?
- refactor (again!)
- add to map chatID level

- change start time to 9:00 next day (remindJobs)
- change period to 1 day
*/

package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Date struct {
	year  int
	month time.Month
	day   int
}

type JItem struct {
	Job  string
	Done bool
}

type Item struct {
	JItem
	JDate Date
}

type State uint8

const dateFormat = "02-01-2006"
const period = 10 * time.Second

const (
	Idle State = iota
	JobWait
	DateWaitAdd
	DateWaitShow
	DateWaitDone
	waitKeyPress
)

var muSendMsg sync.Mutex
var jobs map[Date][]JItem
var subscribers map[int64]chan struct{}
var currState State

var kb struct {
	date  Date
	items []int
	keys  *tgbotapi.ReplyKeyboardMarkup
	show  bool
	shown bool
}

func main() {
	currState = Idle
	tempItem := Item{}
	jobs = make(map[Date][]JItem)
	subscribers = make(map[int64]chan struct{})

	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_APITOKEN"))
	if err != nil {
		panic(err)
	}

	bot.Debug = false

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30
	updates := bot.GetUpdatesChan(updateConfig)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		msgText := ""

		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start":
				msgText = startCmdResponse()
			case "newitem":
				msgText = newItemCmdResponse()
			case "items":
				msgText = itemsCmdResponse()
			case "cancel":
				msgText = cancelCmdResponse()
			case "subscribe":
				msgText = subscribeCmdResponse(bot, update.Message.Chat.ID)
			case "unsubscribe":
				msgText = unsubscribeCmdResponse(update.Message.Chat.ID)
			case "done":
				msgText = doneCmdResponse()
			default:
				msgText = unknownCmdResponse()
			}
		} else {
			switch currState {
			case Idle:
				msgText = onMsgIdleResponse()
			case JobWait:
				msgText = onMsgJobWaitResponse(&tempItem, update.Message.Text)
			case DateWaitAdd:
				msgText, _ = onMsgDateWaitAddResponse(&tempItem, update.Message.Text)
			case DateWaitShow:
				msgText, _ = onMsgDateWaitShowResponse(&tempItem, update.Message.Text)
			case DateWaitDone:
				msgText, _ = onMsgDateWaitDoneResponse(update.Message.Text)
			case waitKeyPress:
				msgText = onMsgWaitKeyPressResponse(update.Message.Text)
			}
		}

		// TODO: refactor me
		if msgText != "" {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
			if kb.show && !kb.shown {
				msg.ReplyMarkup = *(kb.keys)
			}
			if !kb.show && kb.shown {
				msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
			}
			if err := sendMsg(bot, &msg); err != nil {
				log.Printf("Something went wrong: %v\n", err)
			}
			if kb.show && !kb.shown {
				kb.shown = true
			}
			if !kb.show && kb.shown {
				kb.shown = false
			}
		}
	}
}

/*****************************************************************
* User's message handlers
*****************************************************************/

func startCmdResponse() string {
	if currState == Idle {
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

func newItemCmdResponse() string {
	if currState == Idle {
		currState = JobWait
		return "Please Enter new job."
	} else {
		return "Please complete current operation or cancel it using /cancel."
	}
}

func itemsCmdResponse() string {
	if currState == Idle {
		currState = DateWaitShow
		return "Please enter date for which you want to see jobs in format dd-mm-yyyy."
	} else {
		return "Please complete current operation or cancel it using /cancel."
	}
}

func cancelCmdResponse() string {
	if currState != Idle {
		currState = Idle
		return "Current operation was cancelled."
	} else {
		return "I am Idle already."
	}
}

func subscribeCmdResponse(bot *tgbotapi.BotAPI, chatID int64) string {
	var msg string
	if currState == Idle {
		if _, ok := subscribers[chatID]; ok {
			msg = "You are already subscribed. To unsubscribe enter /unsubscribe command."
		} else {
			msg = "You are subscribed now. To unsubscribe enter /unsubscribe command."
			ch := make(chan struct{})
			subscribers[chatID] = ch
			go remindJobs(bot, chatID, period, ch)
		}
	} else {
		msg = "Please complete current operation or cancel it using /cancel."
	}
	return msg
}

func unsubscribeCmdResponse(chatID int64) string {
	var msg string
	if currState == Idle {
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

func doneCmdResponse() string {
	if currState == Idle {
		currState = DateWaitDone
		return "Please enter day where done job is placed in format dd-mm-yyyy."
	} else {
		return "Please complete current operation or cancel it using /cancel."
	}
}

func unknownCmdResponse() string {
	return "I don't know that command. Enter /start to see list of commands."
}

func onMsgIdleResponse() string {
	return "Please enter some command. Enter /start to see list of commands."
}

func onMsgJobWaitResponse(item *Item, answer string) string {
	item.Job = answer
	currState = DateWaitAdd
	return "Please Enter date in format dd-mm-yyyy."
}

func onMsgDateWaitAddResponse(item *Item, answer string) (string, error) {
	date, err := time.Parse(dateFormat, answer)
	if err != nil {
		return "Could not parse date. Try again in format dd-mm-yyyy.", err
	}
	item.JDate = timeToDate(date)
	jobs[item.JDate] = append(jobs[item.JDate], item.JItem)
	currState = Idle
	return "Your job was added.", nil
}

func onMsgDateWaitShowResponse(item *Item, answer string) (string, error) {
	date, err := time.Parse(dateFormat, answer)
	if err != nil {
		return "Could not parse date. Try again in format dd-mm-yyyy.", err
	}
	dateStruct := timeToDate(date)
	currState = Idle
	return getAllJobs(dateStruct), nil
}

func onMsgDateWaitDoneResponse(answer string) (string, error) {
	date, err := time.Parse(dateFormat, answer)
	if err != nil {
		return "Could not parse date. Try again in format dd-mm-yyyy.", err
	}
	dateStruct := timeToDate(date)
	msg, ok := showKeyboard(dateStruct)
	if ok {
		currState = waitKeyPress
	} else {
		currState = Idle
	}
	return msg, nil
}

func onMsgWaitKeyPressResponse(answer string) string {
	num, err := strconv.Atoi(answer)
	if err != nil || num < 0 || num > len(kb.items) {
		return "Press the button on keyboard or type appropriate number."
	}
	kb.show = false
	currState = Idle
	if num == 0 {
		return "Operation canceled."
	} else {
		num--
		jobs[kb.date][kb.items[num]].Done = true
		return "Well done!"
	}

}

/*****************************************************************
* Utility functions
*****************************************************************/
func remindJobs(
	bot *tgbotapi.BotAPI,
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
			if err := sendMsg(bot, &msg); err != nil {
				log.Printf("Something went wrong: %v\n", err)
			}
		case <-done:
			t.Stop()
			return
		}
	}
}

func getAllJobs(t Date) string {
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

func timeToDate(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

func sendMsg(bot *tgbotapi.BotAPI, msg *tgbotapi.MessageConfig) error {
	muSendMsg.Lock()
	_, err := bot.Send(msg)
	muSendMsg.Unlock()
	return err
}

func getDoneStr(b bool) string {
	if b {
		return "Done"
	} else {
		return "TODO"
	}
}

func showKeyboard(date Date) (string, bool) {
	kb.date = date
	kb.items = getTODOlist(date)
	if len(kb.items) == 0 {
		return "There are no undone jobs in this day.", false
	}
	genKeyboard()
	msg := "To choose completed job press appropriate key or 0 for cancel.\n"
	msg += "0. Cancel\n"
	msg += getTODOstring()
	kb.show = true
	return msg, true
}

func getTODOlist(d Date) []int {
	TODOlist := []int{}
	for i, job := range jobs[d] {
		if !job.Done {
			TODOlist = append(TODOlist, i)
		}
	}
	return TODOlist
}

func genKeyboard() {
	var buttons [][]tgbotapi.KeyboardButton
	for i := 0; i < len(kb.items)+1; i++ {
		if len(buttons) <= i/3 {
			buttons = append(buttons, []tgbotapi.KeyboardButton{})
		}
		buttons[i/3] = append(buttons[i/3], tgbotapi.NewKeyboardButton(strconv.Itoa(i)))
	}
	newkb := tgbotapi.NewReplyKeyboard(buttons...)
	kb.keys = &newkb
}

func getTODOstring() string {
	msg := ""
	for i, ind := range kb.items {
		job := jobs[kb.date][ind]
		msg += fmt.Sprintf("%d. %s\n", i+1, job.Job)
	}
	return msg
}
