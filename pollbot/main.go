package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/rs/zerolog"
	"github.com/tarantool/go-tarantool/v2"
)

// --------------------------
// Функции работы с Tarantool
// --------------------------

var conn *tarantool.Connection

// connectToTarantool устанавливает соединение с Tarantool.
func connectToTarantool(addr, user, password string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	dialer := tarantool.NetDialer{
		Address:  addr,
		User:     user,
		Password: password,
	}
	opts := tarantool.Opts{
		Timeout: time.Second,
	}
	var err error
	conn, err = tarantool.Connect(ctx, dialer, opts)
	if err != nil {
		log.Fatalf("Connection failed: %s", err)
	}
	log.Println("Connected to Tarantool")
}

// createPoll вставляет новый опрос в пространство "polls".
func createPoll(id, creator, title string, options []string) error {
	votes := make(map[int]int)
	tuple := []interface{}{id, creator, title, options, votes, true}

	future := conn.Do(tarantool.NewInsertRequest("polls").Tuple(tuple))
	_, err := future.Get()
	if err != nil {
		return fmt.Errorf("failed to create poll: %s", err)
	}
	return nil
}

// votePoll осуществляет голосование: получает опрос, увеличивает голос выбранной опции и заменяет кортеж.
func votePoll(pollID string, optionIndex int) error {
	data, err := conn.Do(
		tarantool.NewSelectRequest("polls").
			Limit(1).
			Iterator(tarantool.IterEq).
			Key([]interface{}{pollID}),
	).Get()
	if err != nil {
		return fmt.Errorf("failed to select poll: %s", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("poll not found")
	}
	tuple, ok := data[0].([]interface{})
	if !ok {
		return fmt.Errorf("invalid tuple format")
	}
	votes, err := convertVotes(tuple[4])
	if err != nil {
		return fmt.Errorf("failed to parse votes: %s", err)
	}
	options, ok := tuple[3].([]interface{})
	if !ok || optionIndex < 0 || optionIndex >= len(options) {
		return fmt.Errorf("invalid option index")
	}
	votes[optionIndex]++
	newTuple := []interface{}{pollID, tuple[1], tuple[2], tuple[3], votes, tuple[5]}
	future := conn.Do(tarantool.NewReplaceRequest("polls").Tuple(newTuple))
	_, err = future.Get()
	if err != nil {
		return fmt.Errorf("failed to update poll: %s", err)
	}
	return nil
}

// dbShowPoll выбирает опрос по ID и формирует строку с результатами.
func dbShowPoll(pollID string) (string, error) {
	data, err := conn.Do(
		tarantool.NewSelectRequest("polls").
			Limit(1).
			Iterator(tarantool.IterEq).
			Key([]interface{}{pollID}),
	).Get()
	if err != nil {
		return "", fmt.Errorf("failed to select poll: %s", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("poll not found")
	}
	tuple, ok := data[0].([]interface{})
	if !ok {
		return "", fmt.Errorf("invalid tuple format")
	}
	optionsRaw, ok := tuple[3].([]interface{})
	if !ok {
		return "", fmt.Errorf("invalid options format")
	}
	options := make([]string, len(optionsRaw))
	for i, o := range optionsRaw {
		str, ok := o.(string)
		if !ok {
			str = fmt.Sprintf("%v", o)
		}
		options[i] = str
	}
	votes, err := convertVotes(tuple[4])
	if err != nil {
		return "", fmt.Errorf("failed to parse votes: %s", err)
	}
	totalVotes := 0
	for _, v := range votes {
		totalVotes += v
	}
	response := fmt.Sprintf("Результаты опроса ID: %s\nВопрос: %s\n", pollID, tuple[2])
	for i, opt := range options {
		count := votes[i]
		percent := 0.0
		if totalVotes > 0 {
			percent = float64(count) / float64(totalVotes) * 100
		}
		response += fmt.Sprintf("%d: %s - %d голосов (%.1f%%)\n", i+1, opt, count, percent)
	}
	return response, nil
}

// stopPoll завершает опрос, устанавливая active = false.
func stopPoll(pollID string) error {
	future := conn.Do(
		tarantool.NewSelectRequest("polls").
			Limit(1).
			Iterator(tarantool.IterEq).
			Key([]interface{}{pollID}),
	)
	data, err := future.Get()
	if err != nil {
		return fmt.Errorf("failed to select poll: %s", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("poll not found")
	}
	tuple, ok := data[0].([]interface{})
	if !ok {
		return fmt.Errorf("invalid tuple format")
	}
	active, ok := tuple[5].(bool)
	if !ok {
		return fmt.Errorf("invalid active flag")
	}
	if !active {
		return fmt.Errorf("poll is already stopped")
	}
	newTuple := []interface{}{pollID, tuple[1], tuple[2], tuple[3], tuple[4], false}
	future = conn.Do(tarantool.NewReplaceRequest("polls").Tuple(newTuple))
	_, err = future.Get()
	if err != nil {
		return fmt.Errorf("failed to stop poll: %s", err)
	}
	return nil
}

// deletePoll удаляет опрос по ID.
func deletePoll(pollID string) error {
	future := conn.Do(
		tarantool.NewDeleteRequest("polls").
			Key([]interface{}{pollID}),
	)
	_, err := future.Get()
	if err != nil {
		return fmt.Errorf("failed to delete poll: %s", err)
	}
	return nil
}

// convertVotes преобразует votes из формата Tarantool в map[int]int.
func convertVotes(votesRaw interface{}) (map[int]int, error) {
	votesMap, ok := votesRaw.(map[interface{}]interface{})
	if !ok {
		// Если карта пустая, возвращаем пустую мапу.
		return make(map[int]int), nil
	}
	votes := make(map[int]int)
	for k, v := range votesMap {
		var keyInt, valueInt int
		switch key := k.(type) {
		case int:
			keyInt = key
		case int8:
			keyInt = int(key)
		default:
			return nil, fmt.Errorf("invalid vote key type")
		}
		switch val := v.(type) {
		case int:
			valueInt = val
		case int8:
			valueInt = int(val)
		default:
			return nil, fmt.Errorf("invalid vote value type")
		}
		votes[keyInt] = valueInt
	}
	return votes, nil
}

// --------------------------
// Функции бота Mattermost
// --------------------------

func main() {

	app := &application{
		logger: zerolog.New(
			zerolog.ConsoleWriter{
				Out:        os.Stdout,
				TimeFormat: time.RFC822,
			},
		).With().Timestamp().Logger(),
	}

	app.config = loadConfig()
	app.logger.Info().Str("config", fmt.Sprint(app.config)).Msg("")

	app.logger.Info().Str("config", fmt.Sprintf("%+v", app.config)).Msg("Loaded configuration")

	setupGracefulShutdown(app)

	connectToTarantool(app.config.tarantoolAddress, app.config.tarantoolUser, app.config.tarantoolPassword)
	defer conn.CloseGraceful()

	app.mattermostClient = model.NewAPIv4Client(app.config.mattermostServer.String())
	app.mattermostClient.SetToken(app.config.mattermostToken)

	// Логинимся.
	user, resp, err := app.mattermostClient.GetUser("me", "")
	if err != nil {
		app.logger.Fatal().Err(err).Msg("Could not log in")
	} else {
		app.logger.Debug().Interface("user", user).Interface("resp", resp).Msg("")
		app.logger.Info().Msg("Logged in to Mattermost")
		app.mattermostUser = user
	}

	// Получаем команду.
	team, resp, err := app.mattermostClient.GetTeamByName(app.config.mattermostTeamName, "")
	if err != nil {
		app.logger.Fatal().Err(err).Msg("Could not find team. Is this bot a member?")
	} else {
		app.logger.Debug().Interface("team", team).Interface("resp", resp).Msg("")
		app.mattermostTeam = team
	}

	// Получаем канал для общения.
	channel, resp, err := app.mattermostClient.GetChannelByName(
		app.config.mattermostChannel, app.mattermostTeam.Id, "",
	)
	if err != nil {
		app.logger.Fatal().Err(err).Msg("Could not find channel. Is this bot added to that channel?")
	} else {
		app.logger.Debug().Interface("channel", channel).Interface("resp", resp).Msg("")
		app.mattermostChannel = channel
	}

	sendMsgToTalkingChannel(app, "Hi! I am a poll bot.", "")

	listenToEvents(app)
}

func setupGracefulShutdown(app *application) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			if app.mattermostWebSocketClient != nil {
				app.logger.Info().Msg("Closing websocket connection")
				app.mattermostWebSocketClient.Close()
			}
			app.logger.Info().Msg("Shutting down")
			os.Exit(0)
		}
	}()
}

// sendMsgToTalkingChannel отправляет сообщение в указанный канал.
func sendMsgToTalkingChannel(app *application, msg string, replyToId string) {
	post := &model.Post{
		ChannelId: app.mattermostChannel.Id,
		Message:   msg,
		RootId:    replyToId,
	}
	if _, _, err := app.mattermostClient.CreatePost(post); err != nil {
		app.logger.Error().Err(err).Str("RootID", replyToId).Msg("Failed to create post")
	}
}

// listenToEvents слушает события Mattermost через websocket.
func listenToEvents(app *application) {
	var err error
	for {
		wsURL := fmt.Sprintf("ws://%s%s", app.config.mattermostServer.Host, app.config.mattermostServer.Path)
		app.mattermostWebSocketClient, err = model.NewWebSocketClient4(wsURL, app.mattermostClient.AuthToken)
		if err != nil {
			app.logger.Warn().Err(err).Msg("Mattermost websocket disconnected, retrying")
			time.Sleep(2 * time.Second)
			continue
		}
		app.logger.Info().Msg("Mattermost websocket connected")
		app.mattermostWebSocketClient.Listen()
		for event := range app.mattermostWebSocketClient.EventChannel {
			go handleWebSocketEvent(app, event)
		}
	}
}

// handleWebSocketEvent обрабатывает входящие события.
func handleWebSocketEvent(app *application, event *model.WebSocketEvent) {
	if event.GetBroadcast().ChannelId != app.mattermostChannel.Id {
		return
	}
	if event.EventType() != model.WebsocketEventPosted {
		return
	}
	post := &model.Post{}
	err := json.Unmarshal([]byte(event.GetData()["post"].(string)), post)
	if err != nil {
		app.logger.Error().Err(err).Msg("Could not cast event to *model.Post")
		return
	}
	if post.UserId == app.mattermostUser.Id {
		return
	}
	handlePollCommand(app, post)
}

// handlePollCommand маршрутизатор команд опроса.
func handlePollCommand(app *application, post *model.Post) {
	args := strings.Fields(post.Message)
	if args[0] != "/poll" {
		return
	}
	if len(args) < 2 {
		sendMsgToTalkingChannel(app, "Некорректная команда для опроса, введите /poll help для вывода существующих команд", post.Id)
		return
	}
	subcommand := args[1]
	switch subcommand {
	case "create":
		handlePollCreate(app, post, args[2:])
	case "vote":
		handlePollVote(app, post, args[2:])
	case "show":
		handlePollShow(app, post, args[2:])
	case "stop":
		handlePollStop(app, post, args[2:])
	case "delete":
		handlePollDelete(app, post, args[2:])
	case "help":
		handlePollHelp(app, post, args[2:])
	default:
		sendMsgToTalkingChannel(app, "Неизвестная подкоманда для /poll, введите /poll help для вывода существующих команд", post.Id)
	}
}

// parsePollData парсит аргументы команды
func parsePollData(args []string) []string {
	re := regexp.MustCompile(`"([^"]+)"|\S+`)
	matches := re.FindAllStringSubmatch(strings.Join(args, " "), -1)
	var parsedArgs []string
	for _, match := range matches {
		if match[1] != "" {
			parsedArgs = append(parsedArgs, match[1])
		} else {
			parsedArgs = append(parsedArgs, match[0])
		}
	}
	return parsedArgs
}

// handlePollCreate создает новый опрос.
func handlePollCreate(app *application, post *model.Post, args []string) {
	parsedArgs := parsePollData(args)
	if len(parsedArgs) < 3 {
		sendMsgToTalkingChannel(app, "Использование: /poll create \"Вопрос\" \"Вариант1\" \"Вариант2\" ...", post.Id)
		return
	}
	pollID := generateUniqueID()
	title := parsedArgs[0]
	options := parsedArgs[1:]
	app.logger.Info().Str("pollID", pollID).Str("creator", post.UserId).Str("title", title).Msg("Creating poll")
	err := createPoll(pollID, post.UserId, title, options)
	if err != nil {
		app.logger.Error().Err(err).Str("pollID", pollID).Msg("Failed to create poll")
		sendMsgToTalkingChannel(app, fmt.Sprintf("Ошибка создания опроса: %s", err), post.Id)
		return
	}
	response := fmt.Sprintf("Опрос создан! ID: %s\nВопрос: %s\nВарианты:\n", pollID, title)
	for i, opt := range options {
		response += fmt.Sprintf("%d: %s\n", i+1, opt)
	}
	app.logger.Info().Str("pollID", pollID).Msg("Poll created successfully")
	sendMsgToTalkingChannel(app, response, post.Id)
}

// handlePollVote регистрирует голос пользователя.
func handlePollVote(app *application, post *model.Post, args []string) {
	if len(args) < 2 {
		sendMsgToTalkingChannel(app, "Для голосования укажите ID опроса и номер варианта", post.Id)
		return
	}
	pollID := args[0]
	optionNumber, err := strconv.Atoi(args[1])
	if err != nil || optionNumber < 1 {
		sendMsgToTalkingChannel(app, "Неверный номер варианта", post.Id)
		return
	}
	app.logger.Info().Str("pollID", pollID).Str("user", post.UserId).Str("option", args[1]).Msg("Voting in poll")
	err = votePoll(pollID, optionNumber-1)
	if err != nil {
		app.logger.Error().Err(err).Str("pollID", pollID).Msg("Failed to vote in poll")
		sendMsgToTalkingChannel(app, fmt.Sprintf("Ошибка голосования: %s", err), post.Id)
		return
	}
	app.logger.Info().Str("pollID", pollID).Msg("Vote recorded successfully")
	sendMsgToTalkingChannel(app, fmt.Sprintf("Ваш голос за вариант %d принят", optionNumber), post.Id)
}

// handlePollShow выводит результаты опроса.
func handlePollShow(app *application, post *model.Post, args []string) {
	if len(args) < 1 {
		sendMsgToTalkingChannel(app, "Укажите ID опроса для показа результатов", post.Id)
		return
	}
	pollID := args[0]
	response, err := dbShowPoll(pollID)
	if err != nil {
		sendMsgToTalkingChannel(app, fmt.Sprintf("Ошибка показа опроса: %s", err), post.Id)
		return
	}
	sendMsgToTalkingChannel(app, response, post.Id)
}

// handlePollStop завершает опрос.
func handlePollStop(app *application, post *model.Post, args []string) {
	if len(args) < 1 {
		sendMsgToTalkingChannel(app, "Укажите ID опроса для остановки", post.Id)
		return
	}
	pollID := args[0]
	app.logger.Info().Str("pollID", pollID).Str("user", post.UserId).Msg("Stopping poll")
	err := stopPoll(pollID)
	if err != nil {
		app.logger.Error().Err(err).Str("pollID", pollID).Msg("Failed to stop poll")
		sendMsgToTalkingChannel(app, fmt.Sprintf("Ошибка остановки опроса: %s", err), post.Id)
		return
	}
	app.logger.Info().Str("pollID", pollID).Str("user", post.UserId).Msg("Poll stopped")
	sendMsgToTalkingChannel(app, "Опрос остановлен", post.Id)
}

// handlePollDelete удаляет опрос.
func handlePollDelete(app *application, post *model.Post, args []string) {
	if len(args) < 1 {
		sendMsgToTalkingChannel(app, "Укажите ID опроса для удаления", post.Id)
		return
	}
	pollID := args[0]
	app.logger.Info().Str("pollID", pollID).Str("user", post.UserId).Msg("Deleting poll")
	err := deletePoll(pollID)
	if err != nil {
		app.logger.Error().Err(err).Str("pollID", pollID).Msg("Failed to delete poll")
		sendMsgToTalkingChannel(app, fmt.Sprintf("Ошибка удаления опроса: %s", err), post.Id)
		return
	}
	app.logger.Info().Str("pollID", pollID).Str("user", post.UserId).Msg("Poll deleted")
	sendMsgToTalkingChannel(app, "Опрос удалён", post.Id)
}

// handlePollHelp выводит справку по командам.
func handlePollHelp(app *application, post *model.Post, args []string) {
	helpText := ""
	if len(args) == 0 {
		helpText = "Доступные команды для опроса:\n" +
			"/poll create \"Вопрос\" \"Вариант1\" \"Вариант2\" ... – создать новый опрос\n" +
			"/poll vote [ID] [номер_варианта] – проголосовать\n" +
			"/poll show [ID] – показать результаты\n" +
			"/poll stop [ID] – остановить опрос (только создатель)\n" +
			"/poll delete [ID] – удалить опрос (только создатель)\n" +
			"/poll help [команда] – справка"
	} else {
		switch args[0] {
		case "create":
			helpText = "/poll create \"Вопрос\" \"Вариант1\" \"Вариант2\" ...\nСоздает новый опрос."
		case "vote":
			helpText = "/poll vote [ID] [номер_варианта]\nГолосует за выбранный вариант."
		case "show":
			helpText = "/poll show [ID]\nПоказывает результаты опроса."
		case "stop":
			helpText = "/poll stop [ID]\nОстанавливает опрос (только создатель)."
		case "delete":
			helpText = "/poll delete [ID]\nУдаляет опрос (только создатель)."
		case "help":
			helpText = "/poll help [команда]\nПоказывает справку по командам."
		default:
			helpText = "Команда не найдена. Доступные команды: create, vote, show, stop, delete, help."
		}
	}
	sendMsgToTalkingChannel(app, helpText, post.Id)
}

func generateUniqueID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
