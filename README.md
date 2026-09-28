# mmvalidator

Веб-приложение на Go для массовой проверки криптографической части кодов маркировки «Честный знак». Один запуск принимает до 400&nbsp;000 непустых строк, устраняет повторы перед запросом и сохраняет все исходные номера строк и признаки.

## Запуск

Нужен Go 1.22 или новее.

```bash
copy .env.example .env
go test ./...
go build -o mmvalidator ./cmd/mmvalidator
./mmvalidator
```

Откройте `http://localhost:8080`. Для Docker: `docker build -t mmvalidator .` и `docker run --env-file .env -p 8080:8080 mmvalidator`.

## Конфигурация

```env
PORT=8080
API_KEY=jwt-token
TRUE_API_URL=https://markirovka.crpt.ru/api/v4/true-api/codes/check
RATE_LIMIT_RPS=1
BATCH_SIZE=1000
HTTP_TIMEOUT_SECONDS=20
```

`RATE_LIMIT_RPS=1` — значение по умолчанию и ограничение на всё приложение, включая повторы запросов. Пакет содержит максимум 1000 кодов. Токен берётся из `API_KEY`, передаётся только как `Authorization: Bearer …`, не записывается в логи и не сохраняется на диск. Если `TRUE_API_URL` оставлен пустым, включается локальный детерминированный mock: строки с `INVALID` считаются невалидными. Это удобно для проверки интерфейса без учётных данных.

Адаптер использует POST `/api/v4/true-api/codes/check` с телом `{"codes":[...]}` и анализирует `verified` в ответе — публичное описание True API, доступное в сообществе, указывает этот метод и формат. Официальная статья также указывает, что криптопроверка ограничивается 2 запросами/сек и 1000 КМ в запросе; проект намеренно работает медленнее — **1 запрос/сек**, согласно вашему уточнению. [Описание запроса и ответа](https://markirovka.ru/community/developers/est-li-kakoe-to-rest-api-dlya-polucheniya-informatsii-o-tovare--90), [ограничения криптопроверки](https://markirovka.ru/community/rezhim-proverok-na-kassakh/proverka-markirovannoy-produktsii-v-moment-priyemki-po-kriteriyami-razreshitelnogo-rezhima-po-api).

Если ваш договор предоставляет именно иной вариант `cises/check`, установите его URL только после сверки его тела и ответа с разделом «Помощь» личного кабинета: публичный контракт его полей не найден, поэтому приложение не выдумывает несовместимый формат. В этом случае замените адаптер `internal/checker/trueapi.go`, сохранив интерфейс `Validator`.

## Вход и результаты

Поддерживаются `.txt` и `.csv`. В TXT: `КОД` либо `КОД ПРИЗНАК`; в CSV первая колонка — код, вторая — признак. CSV читает стандартный Go-парсер, включая quoting. ASCII GS (`0x1D`) в марке сохраняется. В таблице показываются только невалидные коды, а отдельный CSV `check_errors.csv` содержит неуспешно проверенные коды: с невалидными они не смешиваются.

## Устройство

- `internal/parser` — потоковый разбор входа;
- `internal/checker` — интерфейс, mock и HTTP-адаптер с retry/backoff;
- `internal/jobs` — фоновые задачи, дедупликация, прогресс, отмена, CSV;
- `internal/web` — HTTP API, SSE и встроенный статический интерфейс.

HTTP API: `POST /api/jobs`, `GET /api/jobs/{id}`, `GET /api/jobs/{id}/events`, `GET /api/jobs/{id}/invalid`, CSV-маршруты и `DELETE /api/jobs/{id}` для отмены. Завершённые задачи очищаются из памяти через час.
