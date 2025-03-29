## Запуск
Создайте бота на своем mattermost сервере и сохраните его  Access Token. Переименуйте example.env в .env и заполните его необходимыми данными.

Далее введите в терминале docker-compose up --build. Если все сделано правильно то в вашем чате появится сообщение от бота.

![image](https://github.com/user-attachments/assets/94b94832-59b7-43ee-8928-88cfc8c35e61)


## Работа с ботом

Список команд бота можно посмотреть введя команду /poll help (просто текстом, slash command не создавался).


![image](https://github.com/user-attachments/assets/4fb8b3aa-896b-4dca-8c08-2108946c0064)


Пример создания опроса

![image](https://github.com/user-attachments/assets/68372537-e519-47b2-95e7-067ababf2282)

Пример голосования 

![image](https://github.com/user-attachments/assets/a2f5ccbc-63b4-459d-aa4e-3198506dfd61)


Пример просмотра хода голосования

![image](https://github.com/user-attachments/assets/a86c9816-f1f1-4553-adb1-f5a757f5ee86)

Пример остановки опроса

![image](https://github.com/user-attachments/assets/b1394e12-b49b-40a1-bf6f-049faff85cb0)

Пример удаление опроса

![image](https://github.com/user-attachments/assets/4bc1bbd5-f16d-458e-b540-4dd3e90f6b5d)

## Логирование

Логи можно посмотрев введя в консоль docker logs и id контейнера.
