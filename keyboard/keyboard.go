package keyboard

import (
	"bot_module/common"
	"fmt"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var kb struct {
	date  common.Date
	items []int
	keys  *tgbotapi.ReplyKeyboardMarkup
	show  bool
	shown bool
}

func ShowKeyboard(date common.Date, jobs []common.JItem) (string, bool) {
	kb.date = date
	kb.items = getTODOlist(jobs)
	if len(kb.items) == 0 {
		return "There are no undone jobs in this day.", false
	}
	genKeyboard()
	msg := "To choose completed job press appropriate key or 0 for cancel.\n"
	msg += "0. Cancel\n"
	msg += getTODOstring(jobs)
	kb.show = true
	return msg, true
}

func getTODOlist(jobs []common.JItem) []int {
	TODOlist := []int{}
	for i, job := range jobs {
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

func getTODOstring(jobs []common.JItem) string {
	msg := ""
	for i, ind := range kb.items {
		job := jobs[ind]
		msg += fmt.Sprintf("%d. %s\n", i+1, job.Job)
	}
	return msg
}

func IsVisible() bool {
	return kb.shown
}

func Hide() {
	kb.show = false
}

func ItemsCount() int {
	return len(kb.items)
}

func Date() common.Date {
	return kb.date
}

func Index(num int) int {
	return kb.items[num]
}

func VisibilityUpdate() {
	if MustBeShown() {
		kb.shown = true
	}
	if MustBeHidden() {
		kb.shown = false
	}
}

func Keys() tgbotapi.ReplyKeyboardMarkup {
	return *(kb.keys)
}

func MustBeShown() bool {
	return kb.show && !kb.shown
}

func MustBeHidden() bool {
	return !kb.show && kb.shown
}
