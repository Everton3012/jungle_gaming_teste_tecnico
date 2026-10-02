# Jungle Gaming Backend Challenge — Go

Serviço distribuído de carteiras e apostas implementado em Go com Uber Fx, PostgreSQL, AWS SQS via LocalStack e OAuth2/OIDC via Keycloak.

A solução prioriza integridade financeira, idempotência persistente, concorrência entre instâncias, transactional outbox, inbox transacional e recuperação durável de referências pendentes.

## Stack

- Go 1.27.1
- Uber Fx
- `net/http`
- PostgreSQL 17
- `pgx/v5` com SQL explícito
- AWS SDK for Go v2 + SQS FIFO
- LocalStack 4.14
- Keycloak 26.4 (OAuth2/OIDC, `client_credentials`)
- Docker Compose
- migrations versionadas com `migrate/migrate`

## Pré-requisitos

- Docker Desktop / Docker Compose
- Go 1.27.1 para executar a suíte fora do container
- PowerShell 5+ para os scripts em `scripts/` no Windows

## Inicialização

O Compose possui defaults locais seguros para o desafio; `.env` é opcional. Para sobrescrever configurações, copie `.env.example` para `.env`.

Suba o ambiente completo:

```powershell
docker compose up -d --build
```

O Compose:

1. sobe PostgreSQL;
2. sobe LocalStack e cria automaticamente as filas;
3. aplica as migrations;
4. importa o realm do Keycloak;
5. inicia a API somente depois de PostgreSQL/LocalStack/migrations estarem disponíveis.

Serviços locais:

| Serviço | Endereço |
| --- | --- |
| API | `http://localhost:8080` |
| Keycloak | `http://localhost:8085` |
| PostgreSQL | `localhost:5432` |
| LocalStack | `http://localhost:4566` |

Health checks:

```powershell
curl.exe http://localhost:8080/health/live
curl.exe http://localhost:8080/health/ready
curl.exe http://localhost:8080/metrics
```

### Variáveis de ambiente

O Compose possui defaults para checkout limpo. `.env.example` lista os mesmos valores e pode ser copiado para `.env` para override local.

| Variável | Default | Finalidade |
| --- | --- | --- |
| `APP_PORT` | `8080` | porta publicada da API |
| `DATABASE_URL` | PostgreSQL do Compose | conexão `pgx` |
| `DATABASE_MAX_CONNECTIONS` / `DATABASE_MIN_CONNECTIONS` | `20` / `2` | pool PostgreSQL |
| `DATABASE_CONNECT_TIMEOUT` / `DATABASE_OPERATION_TIMEOUT` | `5s` / `5s` | prazos de conexão/operação |
| `REFERENCE_POLL_INTERVAL` | `2s` | polling de referências pendentes |
| `REFERENCE_RETRY_BACKOFF` / `REFERENCE_MAX_BACKOFF` | `5s` / `5m` | backoff exponencial de referência |
| `REFERENCE_MAX_ATTEMPTS` | `10` | expiração de referência ausente |
| `REFERENCE_LEASE_DURATION` | `30s` | lease distribuída do reference worker |
| `OUTBOX_POLL_INTERVAL` | `2s` | polling da outbox |
| `OUTBOX_RETRY_BACKOFF` / `OUTBOX_MAX_BACKOFF` | `5s` / `5m` | retry do publisher |
| `AWS_ENDPOINT_URL` | `http://localstack:4566` | endpoint SQS local |
| `SQS_CONSUMER_QUEUE_NAME` | `wager-transactions.fifo` | fila de entrada |
| `SQS_OUTBOX_QUEUE_NAME` | `wager-events.fifo` | fila de eventos |
| `SQS_CONSUMER_VISIBILITY_TIMEOUT` | `30s` | visibility timeout |
| `AUTH_ISSUER` | realm `jungle` em `localhost:8085` | issuer esperado do JWT |
| `AUTH_JWKS_URL` | Keycloak interno | chaves RS256 |
| `AUTH_WALLET_CLIENT_ID` | `wallet-service` | identidade interna |
| `AUTH_PROVIDER_CLIENTS` | `provider-a,provider-b` | allow-list de provedores |
| `APPLICATION_SHUTDOWN_TIMEOUT` | `10s` | prazo de shutdown |

## OAuth2/OIDC

O realm `jungle` é importado automaticamente a partir de `keycloak/realm-jungle.json`.

Clientes de teste:

| Identidade | Client ID | Client secret | Uso |
| --- | --- | --- | --- |
| Serviço interno | `wallet-service` | `wallet-service-secret` | operações de carteira |
| Provedor A | `provider-a` | `provider-a-secret` | apostas do provider-a |
| Provedor B | `provider-b` | `provider-b-secret` | apostas do provider-b |

Obtenha um token do serviço interno:

```powershell
$walletToken = (Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8085/realms/jungle/protocol/openid-connect/token" `
  -ContentType "application/x-www-form-urlencoded" `
  -Body "grant_type=client_credentials&client_id=wallet-service&client_secret=wallet-service-secret").access_token
```

Token do provider-a:

```powershell
$providerToken = (Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8085/realms/jungle/protocol/openid-connect/token" `
  -ContentType "application/x-www-form-urlencoded" `
  -Body "grant_type=client_credentials&client_id=provider-a&client_secret=provider-a-secret").access_token
```

A identidade autenticada determina o provedor autorizado. Um provider não pode consultar nem processar operações de outro provider. Endpoints de carteira exigem `wallet-service`.

## HTTP API

### Criar carteira

```powershell
$playerId = [guid]::NewGuid().ToString()
$body = @{
  playerId = $playerId
  initialBalance = @{ amount = "100.00"; currency = "BRL" }
} | ConvertTo-Json -Depth 5

$wallet = Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8080/wallets" `
  -Headers @{ Authorization = "Bearer $walletToken" } `
  -ContentType "application/json" `
  -Body $body
```

Resposta:

```json
{
  "id": "...",
  "playerId": "...",
  "balance": { "amount": "100.00", "currency": "BRL" },
  "version": 1
}
```

### Processar aposta

```powershell
$externalId = [guid]::NewGuid().ToString()
$body = @{
  providerId = "provider-a"
  externalTransactionId = $externalId
  playerId = $wallet.playerId
  walletId = $wallet.id
  roundId = "round-1"
  gameId = "fortune-chimp"
  kind = "BET"
  money = @{ amount = "25.00"; currency = "BRL" }
} | ConvertTo-Json -Depth 5

$result = Invoke-RestMethod -Method Post `
  -Uri "http://localhost:8080/wagering/transactions" `
  -Headers @{
    Authorization = "Bearer $providerToken"
    "Idempotency-Key" = "provider-a:$externalId"
  } `
  -ContentType "application/json" `
  -Body $body
```

### Leituras

```text
GET /wallets/{walletId}
GET /wallets/{walletId}/ledger?limit=50&cursor=...
GET /wagering/transactions/{transactionId}
GET /providers/{providerId}/wagering/transactions/{externalTransactionId}
POST /wallets/{walletId}/reconciliation
```

Os endpoints de carteira usam token `wallet-service`. Os endpoints de transação usam token do provider.

### Semântica HTTP

| Situação | Status | Código/corpo |
| --- | ---: | --- |
| operação processada/replay | `200` | `status=PROCESSED`, saldo persistido e `idempotentReplay` |
| carteira criada | `201` | wallet + balance + version |
| referência ainda ausente | `202` | `status=PENDING_REFERENCE` |
| JSON/entrada inválida | `400` | erro estruturado `code/message` |
| bearer ausente/inválido/expirado | `401` | `UNAUTHORIZED` |
| identidade sem permissão/provider mismatch | `403` | `FORBIDDEN`/`PROVIDER_MISMATCH` |
| wallet/transação inexistente | `404` | `WALLET_NOT_FOUND`/`TRANSACTION_NOT_FOUND` |
| chave/ID reutilizado com conteúdo incompatível ou dupla reversão | `409` | conflito estável |
| rejeição definitiva de negócio | `422` | `status=REJECTED` + `failureCode` |
| timeout/infra indisponível | `503` | erro transitório estruturado |

## SQS

Filas provisionadas automaticamente:

- `wager-transactions.fifo`
- `wager-transactions-dlq.fifo`
- `wager-events.fifo`
- filas dedicadas de integração usadas pela suíte

A fila de entrada possui redrive para a DLQ com `maxReceiveCount=5`.

Contrato de entrada:

```json
{
  "messageId": "msg-123",
  "type": "WagerTransactionRequested",
  "occurredAt": "2026-09-08T12:00:00.000Z",
  "data": {
    "providerId": "provider-a",
    "externalTransactionId": "transaction-123",
    "idempotencyKey": "provider-a:transaction-123",
    "playerId": "PLAYER_ID",
    "walletId": "WALLET_ID",
    "roundId": "round-987",
    "gameId": "fortune-chimp",
    "kind": "BET",
    "money": { "amount": "25.00", "currency": "BRL" }
  }
}
```

O `messageId` do envelope é a identidade durável da Inbox. O SQS `MessageId` não substitui essa identidade. A mensagem é removida da fila somente depois do commit durável.

Para FIFO de entrada, use:

- `MessageGroupId`: `walletId`, garantindo serialização natural por carteira no broker sem depender dela para integridade;
- `MessageDeduplicationId`: `messageId`.

A aplicação continua aplicando idempotência persistente e optimistic locking mesmo com FIFO.

## Idempotência

O header `Idempotency-Key` é persistido, mas **não participa do hash de conteúdo**.

HTTP e SQS constroem o mesmo hash SHA-256 a partir de um JSON canônico de campos de negócio normalizados:

- `providerId`
- `externalTransactionId`
- `playerId`
- `walletId`
- `roundId`
- `gameId`
- `kind`
- `money.amount`
- `money.currency`
- `referenceExternalTransactionId`, quando existir

Metadados de transporte (`messageId`, `occurredAt`, correlation/causation) e a chave de idempotência ficam fora do hash.

## Money

`Money` usa `int64` em unidades mínimas (centavos), escala fixa de duas casas e código de moeda. Nenhum cálculo financeiro utiliza `float32`/`float64`.

O tipo trata:

- parsing decimal exato;
- overflow;
- soma/subtração/negação;
- moeda incompatível;
- serialização decimal;
- rejeição de notação científica, NaN/Infinity, escala excedente e valores externos negativos.

## Concorrência

A solução usa optimistic locking na wallet por `version` e retry limitado no caso de uso. O `UPDATE` exige a versão esperada e detecta lost update por `RowsAffected == 0`.

Para executar três instâncias independentes:

```powershell
docker compose --profile multi up -d --build
```

As APIs ficam em `8080`, `8081` e `8082`, cada uma com memória/pool/workers próprios e o mesmo PostgreSQL/SQS.

Execute o cenário obrigatório de duas apostas de 80 sobre 100:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/multi-instance.ps1
```

## Inbox e Outbox

### Inbox

A chave é `(consumer_name, message_id)`. No caminho de produção SQS, Inbox, wallet, transação, ledger e eventos de outbox compartilham a mesma `pgx.Tx`.

Uma reentrega já processada é reconhecida sem repetir o efeito financeiro. Hash diferente para o mesmo `(consumer,messageId)` é conflito.

### Outbox

Eventos financeiros são inseridos na mesma transação da alteração financeira. O publisher usa `FOR UPDATE SKIP LOCKED`, status `PROCESSING` e uma lease (`locked_until`). Registros abandonados após crash voltam a ser elegíveis quando a lease expira. Republicações preservam o mesmo `eventId`.

No SQS de saída:

- `MessageGroupId = aggregateId`;
- `MessageDeduplicationId = eventId`.

## Referências pendentes

`REFUND`/`ROLLBACK` recebidos antes da referência são persistidos como `PENDING_REFERENCE`.

O worker usa estado durável no PostgreSQL:

- contador `reference_attempts`;
- `reference_next_attempt_at`;
- lease `reference_lease_until`;
- backoff exponencial;
- máximo padrão de 10 tentativas.

Após esgotar tentativas, a operação termina em `REJECTED / REFERENCE_NOT_FOUND` e gera `WagerTransactionRejected`.

Referência terminal rejeitada/falhada ou estruturalmente inválida recebe failure code estável correspondente.

## Failure codes principais

- `INSUFFICIENT_FUNDS`
- `REVERSAL_INSUFFICIENT_FUNDS`
- `REFERENCE_NOT_FOUND`
- `REFERENCE_REJECTED`
- `REFERENCE_FAILED`
- `INVALID_REFERENCE`
- `INVALID_AMOUNT`
- `WALLET_MISMATCH`
- `PLAYER_MISMATCH`

## Reconciliação

```text
POST /wallets/{walletId}/reconciliation
```

Executa uma transação read-only `REPEATABLE READ`, reconstrói o saldo exclusivamente a partir do ledger e compara com `wallet.balance`. Não modifica dados.

`difference = storedBalance - calculatedBalance`.

Divergências são expostas na resposta e na métrica `jungle_reconciliation_divergences_total`.

## Observabilidade

Logs são emitidos em JSON através de `slog`.

`GET /metrics` expõe, entre outras:

- resultados processados/rejeitados/pendentes;
- replays idempotentes;
- redeliveries SQS;
- duplicatas de Inbox;
- retries de Outbox;
- conflitos de concorrência;
- divergências de reconciliação;
- latência média de processamento;
- idade do evento de Outbox não publicado mais antigo;
- mensagens aproximadas na DLQ.

Não são logados tokens, secrets nem payloads financeiros completos.

## Migrations

Aplicar:

```powershell
docker compose run --rm migrate
```

As migrations são aplicadas automaticamente no `docker compose up --build`.

Reverter uma migration manualmente:

```powershell
docker compose run --rm migrate -path=/migrations -database "postgres://jungle:jungle@postgres:5432/jungle_gaming?sslmode=disable" down 1
```

Para reconstruir tudo do zero:

```powershell
docker compose down -v --remove-orphans
docker compose up -d --build
```

## Testes

Validação completa em um único comando (reseta volumes locais de teste):

```powershell
powershell -ExecutionPolicy Bypass -File scripts/full-verify.ps1
```

Com PostgreSQL/LocalStack disponíveis e migrations aplicadas:

```powershell
go test ./... -count=1
go test ./... -count=10
go test -race ./... -count=1
go vet ./...
gofmt -l .
go mod verify
```

Smoke autenticado:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/smoke.ps1
```

Três processos independentes:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/multi-instance.ps1
```

Idempotência cruzada SQS → HTTP:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/cross-channel.ps1
```

Retry real e DLQ após cinco entregas:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/dlq.ps1
```

Restart de referência pendente + recuperação de lease da outbox:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/recovery.ps1
```

## Shutdown

Servidor e workers são gerenciados por `fx.Lifecycle`. No encerramento:

- HTTP deixa de aceitar novas entradas;
- contexts dos workers são cancelados;
- mensagem SQS em processamento tem visibility liberada para reentrega se o shutdown ocorrer antes do ACK;
- dependências são encerradas depois dos consumidores;
- erro fatal de background worker solicita shutdown da aplicação para impedir processo aparentemente saudável sem worker.

## Arquitetura

Decisões e trade-offs completos estão em [ARCHITECTURE.md](ARCHITECTURE.md).
