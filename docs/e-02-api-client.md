# E-02. Проверка API-клиента с Prism

В рабочей копии с актуальной `main` запустите Prism по контракту B-01 `api/openapi.yaml`. Команда `make up` поднимает локальное окружение целиком. Чтобы запустить только Prism на `127.0.0.1:4010`, выполните:

```powershell
docker compose --profile mock up mock
```

Без Docker можно запустить Prism из той же рабочей копии:

```powershell
npx --yes @stoplight/prism-cli@5 mock --multiprocess=false -h 127.0.0.1 -p 4010 api/openapi.yaml
```

В рабочей копии с клиентом E-02 запустите проверку. Она отправляет настоящий `GET /api/subjects` через `web/api.js` и проверяет непустой `items` и обязательные поля `Subject`.

```powershell
node tools/check_api_mock.mjs
node --test 'web/**/*.test.mjs'
```

При другом порте передайте адрес первым аргументом `check_api_mock.mjs`. Для проверки из браузера запустите `node web/serve.mjs`, откройте `http://127.0.0.1:8080` и выполните в консоли браузера:

```js
document.cookie = 'sid=mock; Path=/; SameSite=Lax';
const { createApi } = await import('/api.js');
const api = createApi({ baseUrl: 'http://127.0.0.1:4010', credentials: 'include' });
await api.listSubjects();
```

Для локальной проверки используйте одинаковый хост `127.0.0.1` для страницы и Prism. Опция `credentials: 'include'` позволяет отправить тестовую cookie на другой порт. С реальным API `createApi()` использует текущий origin и `credentials: 'same-origin'`. Сессию инициализирует `getMe()` перед вызовом защищённых методов. `request()` предназначен для JSON-методов, а обработка SSE относится к интеграции чата.
