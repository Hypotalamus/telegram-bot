/* TODO:
- add database (?)

- change start time to 9:00 next day (remindJobs)
- change period to 1 day
*/

package main

import (
	"log"
	"os"

	"bot_module/bot"
	"bot_module/keyboard"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	tempItem := bot.Item{}

	tgbot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_APITOKEN"))
	if err != nil {
		panic(err)
	}

	tgbot.Debug = false

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30
	updates := tgbot.GetUpdatesChan(updateConfig)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		msgText := ""

		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start":
				msgText = bot.StartCmdResponse()
			case "newitem":
				msgText = bot.NewItemCmdResponse()
			case "items":
				msgText = bot.ItemsCmdResponse()
			case "cancel":
				msgText = bot.CancelCmdResponse()
			case "subscribe":
				msgText = bot.SubscribeCmdResponse(tgbot, update.Message.Chat.ID)
			case "unsubscribe":
				msgText = bot.UnsubscribeCmdResponse(update.Message.Chat.ID)
			case "done":
				msgText = bot.DoneCmdResponse()
			default:
				msgText = bot.UnknownCmdResponse()
			}
		} else {
			switch bot.CurrState {
			case bot.Idle:
				msgText = bot.OnMsgIdleResponse()
			case bot.JobWait:
				msgText = bot.OnMsgJobWaitResponse(&tempItem, update.Message.Text)
			case bot.DateWaitAdd:
				msgText, _ = bot.OnMsgDateWaitAddResponse(&tempItem, update.Message.Text,
					update.Message.Chat.ID)
			case bot.DateWaitShow:
				msgText, _ = bot.OnMsgDateWaitShowResponse(&tempItem, update.Message.Text,
					update.Message.Chat.ID)
			case bot.DateWaitDone:
				msgText, _ = bot.OnMsgDateWaitDoneResponse(update.Message.Text,
					update.Message.Chat.ID)
			case bot.WaitKeyPress:
				msgText = bot.OnMsgWaitKeyPressResponse(update.Message.Text,
					update.Message.Chat.ID)
			}
		}

		if msgText != "" {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
			if keyboard.MustBeShown() {
				msg.ReplyMarkup = keyboard.Keys()
			}
			if keyboard.MustBeHidden() {
				msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
			}
			if err := bot.SendMsg(tgbot, &msg); err != nil {
				log.Printf("Something went wrong: %v\n", err)
			}
			keyboard.VisibilityUpdate()
		}
	}
}
