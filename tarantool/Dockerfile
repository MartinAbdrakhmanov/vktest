FROM tarantool/tarantool:latest

# Устанавливаем рабочую директорию
WORKDIR /opt/tarantool

# Копируем ваш Lua-скрипт в образ
COPY pollbot.lua .

# Команда запуска Tarantool с вашим скриптом
CMD ["tarantool", "/opt/tarantool/pollbot.lua"]


#  -v /data/dir/on/host:/var/lib/tarantool запускать с тегом docker build -t name . / docker run -p 3301:3301 name 