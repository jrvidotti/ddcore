# Design: Secret Vault (`ddcore.vault` e fieldtype `Vault`)

Registro do desenho arquitetural executado em 2026-09-12 para a [Issue #4](https://github.com/jrvidotti/ddcore/issues/4).
O contrato para desenvolvedores de apps ficará documentado em `docs/agent/vault.md` e `docs/agent/fieldtypes.md`.

---

## 1. Contexto e Motivação

Atualmente, `ddcore.secret(name)` lê `DDCORE_SECRET_<NAME>` das variáveis de ambiente do processo. Isso atende perfeitamente ao modelo de **uma credencial por integração por site** (ex.: chave global do Stripe ou token SMTP da aplicação).

Contudo, para cenários onde a credencial pertence a uma linha (ex.: cada proprietário em uma imobiliária possui sua própria conta e token de API no Asaas), armazenar em variáveis de ambiente exigiria um redeploy a cada novo cliente cadastrado. Por outro lado, armazenar em uma coluna de banco de dados (`Data` ou `Password`) expõe a credencial em:
- Todo backup e `pg_dump`
- Toda réplica de banco
- Todo diff de `Version`
- Todo export de dados

O **Secret Vault** resolve isso criando um cofre gerenciado pelo framework, indexado por chaves textuais dinâmicas, com **criptografia em repouso com chave mestra no ambiente** (`DDCORE_SECRET_KEY`), registro de auditoria em todos os acessos/modificações e uma contraparte no Desk (`fieldtype: "Vault"`) que permite coletar o segredo em formulários sem que o valor jamais se torne uma coluna no banco de dados.

---

## 2. Decisões Arquiteturais

### 2.1 Armazenamento e Criptografia em Repouso
1. **Tabela Interna `ddcore_vault`**:
   Adicionada ao `InternalSchema` em `internal/db/schema.go`:
   ```sql
   CREATE TABLE IF NOT EXISTS ddcore_vault (
     name text PRIMARY KEY,
     ciphertext bytea NOT NULL,
     nonce bytea NOT NULL,
     created timestamptz NOT NULL DEFAULT now(),
     updated timestamptz NOT NULL DEFAULT now()
   );
   ```
2. **Criptografia Autenticada (AES-256-GCM)**:
   - Chave mestra obtida de `os.Getenv("DDCORE_SECRET_KEY")`.
   - Derivação uniforme de 32 bytes via `SHA-256` sobre o segredo da variável de ambiente, aceitando qualquer string arbitrária ou passphrase segura.
   - Nonce aleatório de 12 bytes gerado via `crypto/rand` para cada escrita.
   - Sem a variável `DDCORE_SECRET_KEY`, os dados no Postgres são inacessíveis (um dump do banco sozinho não revela nenhuma credencial).
3. **Ausência da Chave Mestra**:
   - Se `DDCORE_SECRET_KEY` não estiver definida, chamadas a `set`/`get` disparam erro `ValidationError` que nomeia a variável faltante sem vazar valores.

### 2.2 Engine Go e JS Bridge (`ddcore.vault.*`)
1. **Métodos no Engine Go (`internal/engine/vault.go`)**:
   - `e.VaultSet(c *Ctx, name, value string) error`
   - `e.VaultGet(c *Ctx, name string) (string, bool, error)`
   - `e.VaultDel(c *Ctx, name string) error`
   - `e.VaultList(c *Ctx, prefix string) ([]string, error)` (devolve apenas nomes, nunca valores)
   - `e.VaultStatus() (configured bool, count int, names []string, err error)`
2. **Registro de Auditoria**:
   - Toda leitura (`read`), escrita (`write`) e deleção (`delete`) gera um registro no DocType `Vault Audit Log`.
   - Registra: `secret_name`, `action`, `user`, `ip`, `request_id` e timestamp.
   - **Nunca grava o valor do segredo**.
3. **Host e Prelude JS (`internal/engine/host.go` e `internal/js/prelude.js`)**:
   - Mapeia operações `vault.set`, `vault.get`, `vault.del` e `vault.list`.
   - Expõe a API global no servidor (goja):
     ```ts
     ddcore.vault.set("asaas:token:PES-00042", token);
     ddcore.vault.get("asaas:token:PES-00042"); // string | null
     ddcore.vault.del("asaas:token:PES-00042");
     ddcore.vault.list("asaas:token:");         // string[]
     ```
4. **Tipagem SDK (`packages/sdk/src/index.ts`)**:
   - Adicionada interface `vault` a `DDCoreAPI`.

### 2.3 Fieldtype `Vault` e Ciclo de Vida do Documento
1. **Campo Virtual no Meta (`internal/meta/meta.go`)**:
   - `Vault` adicionado a `ValidFieldTypes`.
   - Trato especial em `internal/db/schema.go`: **não gera coluna** em `tab_<doctype>`.
2. **Convenção de Derivação de Chave**:
   - Padrão: `${doctype}:${fieldname}:${doc.name}`.
   - Template opcional em `field.options`: ex.: `"asaas:token:{name}"`, onde `{field}` é interpolado a partir dos valores do documento.
3. **Ciclo de Vida no Save (`insert` e `save`)**:
   - O Engine intercepta campos `Vault` do payload do documento antes de persistir em `tab_<doctype>`.
   - Após persistir a linha (garantindo que `doc.name` foi gerado/atribuído):
     - Novo valor em texto: invoca `VaultSet` com a chave derivada.
     - Marcador de remoção (ex.: `{ clear: true }` ou valor vazio explícito): invoca `VaultDel`.
     - Vazio/null/placeholder `{ configured: true }`: preserva o valor atual sem modificações.
4. **Fronteira e Redação (API, MCP, Export, Version)**:
   - Na leitura de um documento via API ou `c.GetDoc`: o campo `Vault` **nunca** retorna o segredo.
   - Retorna um objeto de status: `{ configured: true }` se a chave existe no vault, ou `null` se não existe.
   - `Version`: campos `Vault` são excluídos da geração de diffs de versão.
   - `Export`: campos `Vault` não são incluídos na exportação de dados.

### 2.4 Desk UI (`desk/src/lib/controls/Control.svelte`)
- Componente de controle para `ft === "Vault"`:
  - Se `value?.configured`: exibe indicador de que o segredo está configurado, botão "Change" (para digitar novo segredo) e botão "Clear" (para remover).
  - Se não configurado: input do tipo `password` para preencher o segredo.

### 2.5 DocType `Vault Audit Log` (`core/doctypes/vault_audit_log/`)
- Módulo `Core`.
- Campos:
  - `secret_name`: Data (searchIndex: true, inListView: true)
  - `action`: Select (`["read", "write", "delete"]`, inListView: true)
  - `user`: Link -> `User` (inListView: true)
  - `ip`: Data
  - `request_id`: Data
- Permissões: `System Manager` com `read`, `report`, `export`, `delete` (para expurgo). Sem permissão de `create` ou `write` (apenas o framework grava).

### 2.6 Diagnóstico (`ddcore doctor`)
- Adicionada seção `Vault` no relatório do `doctor`:
  - Se `DDCORE_SECRET_KEY` configurada: status OK, total de segredos armazenados e listagem das chaves (nomes apenas).
  - Se não configurada: alerta informando que a variável de ambiente precisa ser definida.

---

## 3. Plano de Testes e Validação
1. **Testes Unitários de Criptografia e Engine (`internal/engine/vault_test.go`)**:
   - Criptografia e decriptografia com AES-256-GCM.
   - Recusa quando `DDCORE_SECRET_KEY` não está configurada.
   - UPSERT, deleção e listagem com prefixo.
   - Auditoria de reads, writes e deletes.
   - Isolamento de segredos: `list` nunca devolve valores.
2. **Testes de Integração com DocType e Fieldtype `Vault`**:
   - Inserção de documento com campo `Vault`: valor não grava coluna em `tab_test`, mas persiste cifrado em `ddcore_vault`.
   - Leitura de documento pela API: valor do campo mascarado para `{ configured: true }`.
   - Edição sem alterar o campo `Vault`: segredo permanece inalterado.
   - Edição com novo valor: segredo é atualizado.
   - Edição com limpeza: segredo é removido do vault.
   - Exclusão do documento: segredo correspondente é removido do vault.
   - Ausência em `Version` diff e export.
3. **Testes do Desk e TypeScript**:
   - `npm run check` e `npm test` no desk.
   - `bin/ddcore types` e validação do `@ddcore/sdk`.
