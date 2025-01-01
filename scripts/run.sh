#!/bin/bash

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

APP_BIN_PATH="bin/main"
LOG_FILE="app.log"

echo -e "${YELLOW}Проверка запущенного приложения...${NC}"
APP_PID=$(pgrep -f "$APP_BIN_PATH")

if [ -n "$APP_PID" ]; then
  echo -e "${RED}Найдено запущенное приложение с PID $APP_PID. Останавливаю...${NC}"
  kill -9 "$APP_PID"
  if [ $? -eq 0 ]; then
    echo -e "${GREEN}Приложение успешно остановлено${NC}"
  else
    echo -e "${RED}Не удалось остановить приложение${NC}"
    exit 1
  fi
fi

if [ ! -f "$APP_BIN_PATH" ]; then
  echo -e "${RED}Ошибка: Приложение не скомпилировано или не найдено по пути $APP_BIN_PATH${NC}"
  exit 1
fi

echo -e "${YELLOW}Запуск приложения...${NC}"

echo -e "${YELLOW}Запуск приложения...${NC}"
./"$APP_BIN_PATH" > "$LOG_FILE" 2>&1 &
APP_PID=$!

sleep 2

if ps -p $APP_PID > /dev/null; then
  echo -e "${GREEN}Приложение успешно запущено с PID $APP_PID${NC}"
else
  echo -e "${RED}Ошибка: Приложение завершилось с ошибкой во время запуска${NC}"
  exit 1
fi
