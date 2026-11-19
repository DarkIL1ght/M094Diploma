# E-02. Проверка API-клиента с Prism

Из корня проекта экспортируйте B-01 из актуальной ссылки `origin/main` и запустите Prism. Это пригодится, пока ветка E-02 основана на E-01 и в ней ещё нет каталога `api/`.

```powershell
$contract = Join-Path ([System.IO.Path]::GetTempPath()) 'm094-openapi.yaml'
$yaml = git show origin/main:api/openapi.yaml
if ($LASTEXITCODE -ne 0) { throw 'Контракт B-01 недоступен в origin/main' }
Set-Content -LiteralPath $contract -Value $yaml -Encoding utf8
npx --yes @stoplight/prism-cli@5 mock --multiprocess=false -h 127.0.0.1 -p 4010 $contract
```

В другом терминале запустите проверку. Она отправляет настоящий `GET /api/subjects` через `web/api.js` и проверяет непустой `items` и обязательные поля `Subject`.

```powershell
node tools/check_api_mock.mjs
node --test web/
```

При другом порте передайте адрес первым аргументом `check_api_mock.mjs`. Для проверки из браузера запустите `node web/serve.mjs`, откройте `http://127.0.0.1:8080` и выполните в консоли браузера:

```js
const { createApi } = await import('/api.js');
const api = createApi({ baseUrl: 'http://127.0.0.1:4010', mock: true });
await api.listSubjects();
```

`mock: true` задаёт `sid=mock` для локальной проверки. Используйте один и тот же хост для страницы и Prism. С реальным API `createApi()` использует текущий origin. Сессию инициализирует `getMe()` перед вызовом защищённых методов. `request()` предназначен для JSON-методов, а обработка SSE относится к интеграции чата.
