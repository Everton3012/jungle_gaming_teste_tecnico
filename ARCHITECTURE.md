# Architecture

## Visão geral

A solução separa domínio, aplicação, infraestrutura e adapters de transporte. O domínio não depende de Fx, HTTP, SQS, pgx ou AWS SDK.

```text
HTTP / SQS
    |
    v
Application use cases
    |
    +--> domain (Money, Wallet, WagerTransaction, Ledger rules)
    |
    +--> repositories / transaction manager
              |
              v
          PostgreSQL

Outbox publisher ---> SQS wager-events.fifo
Reference worker ---> pending references in PostgreSQL
OAuth middleware ---> Keycloak JWKS
```

Uber Fx compõe configuração, PostgreSQL, repositórios, casos de uso, auth, HTTP e workers. Recursos de longa duração são gerenciados com `fx.Lifecycle`.

## Money

A representação é `int64` em unidades mínimas. Para BRL, `100.00` é persistido como `10000`.

Razões:

- não há erro binário de ponto flutuante;
- comparação e persistência permanecem exatas;
- `BIGINT` permite constraints explícitas;
- parsing rejeita valores que exigiriam arredondamento silencioso.

O limite é o intervalo de `int64`; parsing e operações detectam overflow.

## Transação financeira

O caso de uso abre uma `pgx.Tx` e deriva todos os repositórios a partir dessa transação. Dentro do mesmo commit ficam, conforme a operação:

- leitura e atualização da wallet;
- criação/atualização de WagerTransaction;
- lançamento imutável do ledger;
- snapshot dos eventos na outbox;
- Inbox, quando a origem é SQS.

Não há commit intermediário de uma operação financeira síncrona.

## Concorrência

A unidade de disputa é a wallet.

A estratégia principal é optimistic locking:

```sql
UPDATE wallets
SET balance = ..., version = version + 1
WHERE id = $1 AND version = $expected;
```

Zero linhas alteradas significa conflito concorrente. O caso de uso executa retry limitado. Não há mutex global ou lock em memória necessário para correção, então processos independentes preservam a mesma garantia.

Constraints do banco continuam sendo a última linha de defesa para saldo não negativo, unicidade e integridade do ledger.

## Ledger

Ledger é append-only.

- `(wallet_id, transaction_id)` é único;
- constraints verificam relação entre before/after/direction;
- triggers impedem UPDATE e DELETE;
- `LOSS` e rejeições não criam lançamentos;
- reversões criam novos lançamentos, nunca editam o original.

## Idempotência

Existem duas identidades persistentes complementares:

1. `(providerId, idempotencyKey)`;
2. `(providerId, externalTransactionId)`.

Replay equivalente retorna o resultado financeiro persistido na transação original. Reuso com conteúdo incompatível gera conflito.

O payload hash é SHA-256 de um JSON canônico contendo somente campos de negócio. A chave de idempotência e metadados HTTP/SQS não entram no hash. Dessa forma, a mesma operação recebida por HTTP e SQS produz o mesmo hash.

## REFUND e ROLLBACK

Referências são resolvidas por `(providerId, referenceExternalTransactionId)` e devem concordar em provider, player, wallet, round, currency e valor.

Política:

- `REFUND` devolve integralmente BET processada;
- `ROLLBACK` aplica o movimento oposto de BET, WIN ou REFUND;
- reversões parciais são rejeitadas;
- constraints/índices impedem dupla reversão bem-sucedida do mesmo tipo;
- débito impossível em uma reversão usa `REVERSAL_INSUFFICIENT_FUNDS`, separado de `INSUFFICIENT_FUNDS` de BET.

## Referências pendentes

Se a referência ainda não existe, a operação entra em `PENDING_REFERENCE` dentro do mesmo commit que grava seu evento de pendência.

O worker utiliza:

- `reference_attempts`;
- `reference_next_attempt_at`;
- `reference_lease_until`;
- `FOR UPDATE SKIP LOCKED`;
- retry exponencial limitado por `REFERENCE_MAX_BACKOFF`;
- lease para recuperação de worker morto;
- `REFERENCE_MAX_ATTEMPTS` para expiração.

Após o limite, a transação é finalizada em `REJECTED` com `REFERENCE_NOT_FOUND` e outbox de rejeição.

Referência ainda pendente é reagendada. Referência terminal rejeitada/falhada ou incompatível encerra a operação com failure code específico.

## Inbox SQS

A identidade é `(consumer_name, message_id)`. Para o consumidor de apostas:

```text
consumer_name = wager-transactions
message_id    = envelope.messageId
```

A Inbox armazena também hash, estado, tentativas e timestamps.

No caminho de produção:

```text
BEGIN
  Inbox Begin/dedup
  processamento financeiro
  ledger
  outbox
  Inbox PROCESSED
COMMIT
DeleteMessage(SQS)
```

Logo, não existe janela em que o ACK seja enviado antes do commit durável.

Se o processo cair depois do commit e antes do `DeleteMessage`, a mensagem é entregue novamente e a Inbox retorna `AlreadyProcessed` sem reaplicar dinheiro.

Mensagens inválidas não são ACKadas; após redeliveries atingem a DLQ configurada no broker.

## SQS FIFO

Entrada:

- `MessageGroupId = walletId` é a recomendação para manter ordem por wallet;
- `MessageDeduplicationId = messageId`.

Saída:

- `MessageGroupId = aggregateId`;
- `MessageDeduplicationId = eventId`.

A correção não depende da deduplicação FIFO. PostgreSQL continua responsável por idempotência e integridade.

## Transactional Outbox

A outbox é persistida no mesmo commit da operação que originou o evento.

Publishers disputam registros com `FOR UPDATE SKIP LOCKED`. Ao assumir um evento, ele passa a `PROCESSING`, recebe `locked_until` e incrementa tentativas.

Se o publisher morrer:

- antes do envio: outro publisher recupera o registro após a lease;
- depois do envio e antes de `PUBLISHED`: o evento pode ser republicado, mas mantém o mesmo `eventId`, permitindo deduplicação downstream/FIFO.

Falhas de publicação retornam o evento para `PENDING` com backoff exponencial.

## Eventos

Eventos obrigatórios produzidos:

- `WagerTransactionProcessed`;
- `WagerTransactionRejected`;
- `WalletBalanceChanged`;
- `WagerTransactionPendingReference`.

O payload salvo na outbox é snapshot imutável. Money é serializado em string decimal.

## Autenticação e autorização

O IdP é Keycloak. A aplicação não cadastra senha nem emite tokens.

Fluxo local: OAuth2 `client_credentials`.

Identidades:

- `wallet-service`: autorização interna para wallet/read/reconciliation;
- `provider-a` e `provider-b`: autorização de provider.

A API valida JWT RS256 usando JWKS do Keycloak, valida issuer e expiração e deriva a autorização a partir de `azp` (`client_id`).

O `providerId` efetivo não é confiado ao JSON enviado pelo cliente. Para POST, ele é substituído pela identidade autenticada após rejeitar mismatch. Para leituras, a transação é devolvida somente se pertencer ao provider autenticado.

Em produção, o mesmo modelo deve usar TLS, secrets externos e políticas IAM/broker. LocalStack usa credenciais explícitas de desenvolvimento para emular a fronteira AWS.

## Fx e lifecycle

Módulos:

- config;
- PostgreSQL;
- application;
- HTTP.

HTTP, reference worker, outbox publisher e wager SQS consumer usam lifecycle. Workers recebem `context.Context` cancelável.

Erro fatal de worker solicita shutdown Fx; a aplicação não permanece aparentemente saudável após perder um worker crítico.

No SIGTERM, o consumer deixa de buscar novas mensagens. Se o context for cancelado durante uma mensagem antes do ACK, tenta zerar sua visibility com context independente e timeout curto para permitir reentrega.

## Reconciliação

A reconciliação usa transação `REPEATABLE READ`, read-only. O saldo é reconstruído pela sequência completa de créditos e débitos do ledger e comparado com o saldo armazenado.

```text
difference = stored - calculated
```

Não há correção automática. Divergência permanece auditável, é retornada ao cliente e incrementa métrica.

## Observabilidade

Logs estruturados JSON são emitidos por `slog`.

Identificadores são incluídos quando disponíveis, sem registrar tokens/secrets/payload financeiro completo.

Métricas expostas em `/metrics` incluem status de operação, replay, redelivery, Inbox duplicate, retries Outbox, conflitos, reconciliação, latência, lag da outbox e DLQ.

`/health/live` valida vida do processo. `/health/ready` valida PostgreSQL e filas SQS necessárias.

## Migrations

- `000001`: entidades financeiras/constraints/ledger;
- `000002`: transactional outbox;
- `000003`: Inbox;
- `000004`: consumer-scoped Inbox, lease da outbox e retry/lease de pending references.

Todas possuem `up` e `down`.

## Três processos independentes

O profile Compose `multi` sobe três containers da API (`8080`, `8081`, `8082`). Cada container possui processo, heap, Fx graph, pool PostgreSQL e workers próprios. Essa configuração é usada pelo script `scripts/multi-instance.ps1` para o cenário 100 - 80 - 80.

## Limitações deliberadas

- Métricas são expostas no formato textual Prometheus, mas sem biblioteca/registry externo para manter dependências pequenas.
- LocalStack não replica integralmente políticas IAM reais; credenciais AWS são explícitas no ambiente local. Em produção, a mesma fronteira deve usar roles/policies do broker.
- O projeto usa IDs UUID-v4-like gerados com `crypto/rand`; ordenação de ledger não depende do UUID, e sim `(created_at,id)`.
- Não há tracing distribuído OpenTelemetry porque é diferencial opcional.
