box.cfg{listen = 3301}


box.once("bootstrap", function()
    if not box.schema.user.exists('polluser') then
        box.schema.user.create("polluser", { password = os.getenv("TT_PASSWORD") or "pass" })
        box.schema.user.grant("polluser", "read,write,execute", "universe")
    end
end)

-- Создаем пространство для опросов
polls_space = box.schema.space.create('polls', {
    if_not_exists = true,
    engine = 'memtx', 
})

-- Создаем структуру таблицы
polls_space:format({
    {name = 'id', type = 'string'},         -- ID опроса
    {name = 'creator', type = 'string'},    -- Создатель опроса
    {name = 'title', type = 'string'},      -- Заголовок (вопрос)
    {name = 'options', type = 'array'},     -- Опции голосования (массив строк)
    {name = 'votes', type = 'map'},         -- Голоса (отображение индекс -> количество голосов)
    {name = 'active', type = 'boolean'},    -- Статус активности
})

-- Создаем индексы для быстрого поиска
-- Первичный индекс по ID опроса
polls_space:create_index('primary', {
    type = 'hash',
    parts = {'id'},
    if_not_exists = true
})

