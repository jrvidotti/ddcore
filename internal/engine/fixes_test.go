package engine

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
)

// B06 — ddcore.db.sql precisa ser somente leitura de verdade: uma CTE de
// escrita disfarçada de SELECT tem de ser recusada pelo banco e nada pode
// ficar gravado.
func TestB06_SQLReadonlyRecusaEscrita(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := c.SQL(`WITH changed AS (
			UPDATE tab_role SET modified_by = 'audit' WHERE name = $1 RETURNING name
		) SELECT * FROM changed`, []any{"Gestor"})
		if err == nil {
			t.Fatalf("esperava recusa da CTE de escrita")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "read-only") {
			t.Fatalf("esperava erro de transação somente leitura, veio: %v", err)
		}
		// a transação continua utilizável e o SET LOCAL foi desfeito
		rows, err := c.SQL(`SELECT modified_by FROM tab_role WHERE name = $1`, []any{"Gestor"})
		if err != nil {
			t.Fatalf("SELECT após a recusa falhou: %v", err)
		}
		if len(rows) != 1 || db.Str(rows[0]["modified_by"]) == "audit" {
			t.Fatalf("a escrita vazou: %v", rows)
		}
		// escrita legítima pelo caminho normal continua funcionando
		_, err = c.DBSet("Role", "Gestor", Doc{"role_name": "Gestor"}, true)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	e.Run(ctx, "Administrator", func(c *Ctx) error {
		v, _ := c.GetValue("Role", "Gestor", "modified_by")
		if db.Str(v) == "audit" {
			t.Fatalf("modified_by foi gravado pela CTE")
		}
		return nil
	})
}

// B07 — duas transações reais gravando o mesmo documento: T1 altera
// full_name e segura a transação; T2 leu a versão anterior e salva apenas
// language. A edição de T1 não pode se perder.
func TestB07_LostUpdate(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		u, _ := c.NewDoc("User", Doc{"email": "co@x.com", "full_name": "Original"})
		_, err := c.Insert(u, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// T2 lê a versão antiga antes de T1 gravar
	var stale Doc
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		var err error
		stale, err = c.GetDoc("User", "co@x.com")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	t1Saved, t1Commit, t1Done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		t1Done <- e.Run(ctx, "Administrator", func(c *Ctx) error {
			d, err := c.GetDoc("User", "co@x.com")
			if err != nil {
				return err
			}
			d["full_name"] = "Primeiro"
			if _, err := c.Save(d, SaveOpts{}); err != nil {
				return err
			}
			close(t1Saved)
			<-t1Commit // segura a transação aberta
			return nil
		})
	}()
	<-t1Saved

	t2Done := make(chan error, 1)
	go func() {
		t2Done <- e.Run(ctx, "Administrator", func(c *Ctx) error {
			d := stale.Clone()
			d["language"] = "en"
			_, err := c.Save(d, SaveOpts{})
			return err
		})
	}()
	select {
	case err := <-t2Done:
		t.Fatalf("T2 não esperou o lock de T1: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	close(t1Commit)
	if err := <-t1Done; err != nil {
		t.Fatalf("T1: %v", err)
	}
	err := <-t2Done
	if err == nil {
		t.Fatalf("T2 gravou sobre a versão antiga sem reclamar")
	}
	if got := cerr.From(err).Type; got != "TimestampMismatchError" {
		t.Fatalf("esperava TimestampMismatchError, veio %s (%v)", got, err)
	}
	e.Run(ctx, "Administrator", func(c *Ctx) error {
		d, err := c.GetDoc("User", "co@x.com")
		if err != nil {
			return err
		}
		if d.Str("full_name") != "Primeiro" {
			t.Fatalf("a edição de T1 se perdeu: %v", d.Str("full_name"))
		}
		return nil
	})
}

// B07 — timestamp inválido não pode passar como "igual".
func TestB07_TimestampInvalidoRecusado(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		u, _ := c.NewDoc("User", Doc{"email": "ts@x.com", "full_name": "TS"})
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		d, err := c.GetDoc("User", "ts@x.com")
		if err != nil {
			return err
		}
		d["modified"] = "nao é uma data"
		d["full_name"] = "Outro"
		if _, err := c.Save(d, SaveOpts{}); err == nil || cerr.From(err).Type != "TimestampMismatchError" {
			t.Fatalf("esperava TimestampMismatchError, veio %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// B03 — salvar um pai com uma linha filha que pertence a outro documento não
// pode transferir a linha: ela vira uma cópia.
func TestB03_FilhoNaoMudaDePai(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		a, _ := c.NewDoc("User", Doc{"email": "a@x.com", "full_name": "A"})
		a["roles"] = []any{map[string]any{"role": "Gestor"}}
		a, err := c.Insert(a, SaveOpts{})
		if err != nil {
			return err
		}
		b, _ := c.NewDoc("User", Doc{"email": "b@x.com", "full_name": "B"})
		b, err = c.Insert(b, SaveOpts{})
		if err != nil {
			return err
		}
		linhaDeA := a.Children("roles")[0]
		nomeOriginal := linhaDeA.Str("name")
		if nomeOriginal == "" {
			t.Fatalf("linha filha sem name: %v", linhaDeA)
		}
		// copia a linha de A para B mantendo o name
		b["roles"] = []any{map[string]any(linhaDeA.Clone())}
		b, err = c.Save(b, SaveOpts{})
		if err != nil {
			return err
		}
		a, err = c.GetDoc("User", "a@x.com")
		if err != nil {
			return err
		}
		if len(a.Children("roles")) != 1 {
			t.Fatalf("A perdeu a linha filha: %v", a["roles"])
		}
		if a.Children("roles")[0].Str("name") != nomeOriginal {
			t.Fatalf("a linha de A trocou de name")
		}
		if len(b.Children("roles")) != 1 {
			t.Fatalf("B deveria ter uma cópia: %v", b["roles"])
		}
		if b.Children("roles")[0].Str("name") == nomeOriginal {
			t.Fatalf("B ficou com a mesma linha de A (%s)", nomeOriginal)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// B21 — depois de um dbSet, o documento em memória precisa carregar o novo
// `modified`; senão um save() na mesma instância falha com TimestampMismatch
// sem ninguém ter editado o documento. E runMethod devolve a versão
// persistida.
func TestB21_DbSetAtualizaModified(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		c.Flags["ignorePermissions"] = true
		p, _ := c.NewDoc("Pessoa", Doc{"nome": "Cli", "tipo": "PF"})
		if _, err := c.Insert(p, SaveOpts{}); err != nil {
			return err
		}
		ped, _ := c.NewDoc("Pedido", Doc{"cliente": "Cli"})
		ped, err := c.Insert(ped, SaveOpts{})
		if err != nil {
			return err
		}
		rt, err := c.RT()
		if err != nil {
			return err
		}
		res, err := rt.RunMethod("Pedido", "tocar", ped.JSON(), []byte(`{}`))
		if err != nil {
			t.Fatalf("dbSet seguido de save falhou: %v", err)
		}
		var out Doc
		if err := json.Unmarshal(res.Doc, &out); err != nil {
			return err
		}
		atual, err := c.GetDoc("Pedido", ped.Name())
		if err != nil {
			return err
		}
		if out.Str("obs") != "tocado" || atual.Str("obs") != "tocado" {
			t.Fatalf("obs não foi gravado: %v / %v", out["obs"], atual["obs"])
		}
		if !sameTime(out["modified"], atual["modified"], time.UTC) {
			t.Fatalf("runMethod devolveu modified desatualizado: %v != %v", out["modified"], atual["modified"])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// B14 — índices únicos: o predicado precisa combinar com o tipo da coluna
// (`<> ”` só em text) e mudar searchIndex ↔ unique precisa recriar o índice,
// já que o nome é o mesmo.
func TestB14_IndicesUnicos(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	indexdef := func(name string) string {
		var out string
		e.Run(ctx, "Administrator", func(c *Ctx) error {
			rows, err := c.SQL(`SELECT indexdef FROM pg_indexes WHERE indexname = $1`, []any{name})
			if err != nil {
				return err
			}
			if len(rows) > 0 {
				out = db.Str(rows[0]["indexdef"])
			}
			return nil
		})
		return out
	}
	migrar := func(passo string) {
		if _, err := e.Migrate(ctx, false); err != nil {
			t.Fatalf("%s: migrate falhou: %v", passo, err)
		}
		plan, err := e.Plan(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(plan) != 0 {
			t.Fatalf("%s: migrate não ficou idempotente: %v", passo, plan)
		}
	}

	// Currency unique: o predicado não pode comparar numeric com ''
	pessoa, _ := e.Meta.Get("Pessoa")
	pessoa.Field("limite").Unique = true
	migrar("currency unique")
	if def := indexdef("tab_pessoa_limite"); !strings.Contains(def, "UNIQUE") || strings.Contains(def, "''") {
		t.Fatalf("índice de Currency inesperado: %q", def)
	}

	// Link com índice de busca vira unique e depois volta
	pedido, _ := e.Meta.Get("Pedido")
	antes := indexdef("tab_pedido_cliente")
	if antes == "" || strings.Contains(antes, "UNIQUE") {
		t.Fatalf("esperava índice não único em cliente: %q", antes)
	}
	pedido.Field("cliente").Unique = true
	migrar("link → unique")
	if def := indexdef("tab_pedido_cliente"); !strings.Contains(def, "UNIQUE") {
		t.Fatalf("índice de Link não virou unique: %q", def)
	}
	pedido.Field("cliente").Unique = false
	migrar("unique → link")
	if def := indexdef("tab_pedido_cliente"); def == "" || strings.Contains(def, "UNIQUE") {
		t.Fatalf("índice unique não voltou a ser de busca: %q", def)
	}
}

// ddcore.db.lock serializa duas transações que pedem a mesma chave.
func TestDBLockSerializa(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var mu sync.Mutex
	var ordem []string
	primeiroSaiu := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		e.Run(ctx, "Administrator", func(c *Ctx) error {
			if err := c.Lock("contrato:1"); err != nil {
				t.Error(err)
				return err
			}
			close(primeiroSaiu)
			time.Sleep(400 * time.Millisecond)
			mu.Lock()
			ordem = append(ordem, "t1")
			mu.Unlock()
			return nil
		})
	}()
	<-primeiroSaiu
	time.Sleep(50 * time.Millisecond)
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		if err := c.Lock("contrato:1"); err != nil {
			return err
		}
		mu.Lock()
		ordem = append(ordem, "t2")
		mu.Unlock()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(ordem) != 2 || ordem[0] != "t1" || ordem[1] != "t2" {
		t.Fatalf("lock não serializou: %v", ordem)
	}
}

// B19 — um filtro em campo de filho não pode duplicar o pai na listagem nem
// na contagem, e Count precisa enxergar os mesmos orFilters da listagem.
func TestB19_FiltroEmFilhoNaoDuplica(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		if _, err := c.Insert(mustDoc(t, c, "Pessoa", Doc{"nome": "Cli", "cpf": "900"}), SaveOpts{}); err != nil {
			return err
		}
		ped := mustDoc(t, c, "Pedido", Doc{"cliente": "Cli"})
		// duas linhas filhas casam com o mesmo filtro
		ped["itens"] = []any{
			map[string]any{"descricao": "Cadeira", "qtd": 1, "valor": 10},
			map[string]any{"descricao": "Cadeira", "qtd": 2, "valor": 20},
		}
		_, err := c.Insert(ped, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		f := []any{[]any{"Item Pedido", "descricao", "=", "Cadeira"}}
		rows, err := c.GetList("Pedido", ListArgs{Filters: f, Fields: []string{"name"}})
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			t.Fatalf("esperava 1 pedido, veio %d: %v", len(rows), rows)
		}
		n, err := c.Count("Pedido", f)
		if err != nil {
			return err
		}
		if n != 1 {
			t.Fatalf("count duplicou o pai: %d", n)
		}
		// duas condições no mesmo filho exigem a mesma linha
		n, err = c.Count("Pedido", []any{
			[]any{"Item Pedido", "descricao", "=", "Cadeira"},
			[]any{"Item Pedido", "qtd", "=", 5},
		})
		if err != nil {
			return err
		}
		if n != 0 {
			t.Fatalf("as condições do filho deveriam valer para a mesma linha: %d", n)
		}
		// orFilters chegam à contagem
		n, err = c.Count("Pedido", nil, []any{[]any{"cliente", "=", "ninguém"}})
		if err != nil {
			return err
		}
		if n != 0 {
			t.Fatalf("Count ignorou orFilters: %d", n)
		}
		n, err = c.Count("Pedido", nil, []any{[]any{"cliente", "=", "Cli"}})
		if err != nil {
			return err
		}
		if n != 1 {
			t.Fatalf("Count com orFilters: %d", n)
		}
		// filho que não é tabela do pai é recusado
		if _, err := c.Count("Pessoa", []any{[]any{"Item Pedido", "descricao", "=", "x"}}); err == nil {
			t.Fatalf("esperava recusa de filho não vinculado")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustDoc(t *testing.T, c *Ctx, dt string, values Doc) Doc {
	t.Helper()
	d, err := c.NewDoc(dt, values)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// B08 — requisições concorrentes durante e.Load(): cada ctx precisa ver meta,
// pool e traduções da mesma geração, sem data race e sem devolver runtime
// antigo ao pool novo.
func TestB08_ReloadKeepsPoolConsistent(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := c.Insert(mustDoc(t, c, "Pessoa", Doc{"nome": "Reload", "cpf": "808"}), SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	errs := make(chan error, 16)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				err := e.Run(ctx, "Administrator", func(c *Ctx) error {
					if c.St != c.E.Current() && c.St == nil {
						t.Error("ctx sem estado")
					}
					rows, err := c.GetList("Pessoa", ListArgs{Filters: map[string]any{"nome": "Reload"}})
					if err != nil {
						return err
					}
					if len(rows) != 1 {
						t.Errorf("listagem inconsistente durante reload: %v", rows)
					}
					rt, err := c.RT()
					if err != nil {
						return err
					}
					if _, err := rt.Meta(); err != nil {
						return err
					}
					_ = c.T("Nome")
					return nil
				})
				if err != nil {
					select {
					case errs <- err:
					default:
					}
					return
				}
			}
		}()
	}
	for i := 0; i < 4; i++ {
		if err := e.Load(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
	select {
	case err := <-errs:
		t.Fatalf("requisição falhou durante reload: %v", err)
	default:
	}

	// depois do reload o ctx novo usa o pool novo e o antigo foi retirado
	st := e.Current()
	c := e.NewCtx(ctx, "Administrator")
	if c.St != st {
		t.Fatalf("NewCtx não capturou o estado corrente")
	}
	if e.Meta != st.Meta || e.Pool != st.Pool {
		t.Fatalf("campos legados divergiram do estado corrente")
	}
}

// B15 — um job que não retorna precisa ser interrompido pelo timeout, e um
// job "running" cujo worker morreu volta para a fila quando o lease vence.
func TestB15_JobTimeout(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	inicio := time.Now()
	_, err := e.RunJob(withTimeout(ctx, 2*time.Second), "Administrator", "demo.services.loop.travar", nil)
	if err == nil {
		t.Fatalf("esperava erro de timeout")
	}
	if d := time.Since(inicio); d > 10*time.Second {
		t.Fatalf("o job só parou depois de %s", d)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("erro inesperado: %v", err)
	}

	// a VM continua utilizável depois da interrupção
	res, err := e.RunJob(ctx, "Administrator", "demo.services.loop.ok", map[string]any{"x": 7})
	if err != nil || !strings.Contains(string(res), `"x":7`) {
		t.Fatalf("runtime não sobreviveu ao interrupt: %s %v", res, err)
	}
}

func withTimeout(ctx context.Context, d time.Duration) context.Context {
	c, cancel := context.WithTimeout(ctx, d)
	_ = cancel
	return c
}

// B15 — worker morto: o job fica running com lease vencido e volta à fila.
func TestB15_LeaseVencidoVoltaParaFila(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var id int64
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		var err error
		id, err = c.Enqueue("demo.services.loop.ok", nil, map[string]any{"timeout": 5})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// simula o claim de um worker que morreu logo depois
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET status = 'running', attempts = 1, lease_until = now() - interval '1 minute' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if err := e.requeueStale(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := e.DB.Pool.QueryRow(ctx, `SELECT status FROM ddcore_job WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("job abandonado não voltou para a fila: %s", status)
	}
	// esgotadas as tentativas, vira failed em vez de rodar para sempre
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET status = 'running', attempts = max_attempts, lease_until = now() - interval '1 minute' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if err := e.requeueStale(ctx); err != nil {
		t.Fatal(err)
	}
	e.DB.Pool.QueryRow(ctx, `SELECT status FROM ddcore_job WHERE id = $1`, id).Scan(&status)
	if status != "failed" {
		t.Fatalf("esperava failed depois de esgotar as tentativas: %s", status)
	}
	// timeout_seconds gravado no enqueue
	var to int
	e.DB.Pool.QueryRow(ctx, `SELECT timeout_seconds FROM ddcore_job WHERE id = $1`, id).Scan(&to)
	if to != 5 {
		t.Fatalf("timeout do job não foi gravado: %d", to)
	}
}

// Lacuna: checkAllowOnSubmit rodava antes de validate/beforeSave, então um
// hook ainda conseguia alterar campo protegido depois da checagem.
func TestAllowOnSubmitDepoisDosHooks(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		if _, err := c.Insert(mustDoc(t, c, "Pessoa", Doc{"nome": "Sub", "cpf": "777"}), SaveOpts{}); err != nil {
			return err
		}
		ped := mustDoc(t, c, "Pedido", Doc{"cliente": "Sub", "desconto": 1})
		ped["itens"] = []any{map[string]any{"descricao": "a", "qtd": 1, "valor": 10}}
		ped, err := c.Insert(ped, SaveOpts{})
		if err != nil {
			return err
		}
		if ped, err = c.Submit(ped); err != nil {
			return err
		}
		// obs é allowOnSubmit, mas o hook mexe em desconto, que não é
		ped["obs"] = "bagunca"
		if _, err := c.Save(ped, SaveOpts{}); err == nil || !strings.Contains(err.Error(), "cannot be changed after submission") {
			t.Fatalf("hook alterou campo protegido depois do envio: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Lacuna: readOnlyDependsOn precisa valer no servidor — esconder o campo na
// tela não é autorização.
func TestReadOnlyDependsOnNoServidor(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		p, err := c.Insert(mustDoc(t, c, "Pessoa", Doc{"nome": "RO", "cpf": "555", "tipo": "PF", "codigo": "A"}), SaveOpts{})
		if err != nil {
			return err
		}
		// PF: o campo é editável
		p["codigo"] = "B"
		if p, err = c.Save(p, SaveOpts{}); err != nil {
			return err
		}
		p["tipo"] = "PJ"
		if p, err = c.Save(p, SaveOpts{}); err != nil {
			return err
		}
		// PJ: a expressão é verdadeira, o campo não pode mudar
		p["codigo"] = "C"
		if _, err := c.Save(p, SaveOpts{}); err == nil || !strings.Contains(err.Error(), "is read-only") {
			t.Fatalf("readOnlyDependsOn não foi imposto: %v", err)
		}
		// salvar sem mexer no campo continua funcionando
		p, _ = c.GetDoc("Pessoa", "RO")
		p["limite"] = 10
		_, err = c.Save(p, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// B04 — o diff da Version não pode carregar senha nem hash.
func TestB04_VersaoNaoGuardaSegredo(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		p, err := c.Insert(mustDoc(t, c, "Pessoa", Doc{"nome": "Seg", "cpf": "444", "segredo": "abc"}), SaveOpts{})
		if err != nil {
			return err
		}
		p["segredo"], p["limite"] = "xyz", 5
		if _, err := c.Save(p, SaveOpts{}); err != nil {
			return err
		}
		rows, err := c.GetList("Version", ListArgs{Filters: map[string]any{"ref_doctype": "Pessoa", "docname": "Seg"}, Fields: []string{"data"}, IgnorePermissions: true})
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			t.Fatalf("esperava 1 versão, veio %d", len(rows))
		}
		data := db.Str(rows[0]["data"])
		if strings.Contains(data, "segredo") || strings.Contains(data, "xyz") {
			t.Fatalf("a versão guardou o segredo: %s", data)
		}
		if !strings.Contains(data, "limite") {
			t.Fatalf("a versão deveria registrar limite: %s", data)
		}

		// User: password_hash é Data, mas continua fora do histórico
		u, err := c.Insert(mustDoc(t, c, "User", Doc{"email": "seg@x.com", "full_name": "Seg", "new_password": "segredo1"}), SaveOpts{})
		if err != nil {
			return err
		}
		u["new_password"] = "segredo2"
		if _, err := c.Save(u, SaveOpts{}); err != nil {
			return err
		}
		vs, err := c.GetList("Version", ListArgs{Filters: map[string]any{"ref_doctype": "User", "docname": "seg@x.com"}, Fields: []string{"data"}, IgnorePermissions: true})
		if err != nil {
			return err
		}
		for _, v := range vs {
			if strings.Contains(db.Str(v["data"]), "password") {
				t.Fatalf("versão de User vazou senha: %s", db.Str(v["data"]))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Lacuna: `requires` precisa ser validado e ordenado no Load.
func TestRequiresOrdenaEValida(t *testing.T) {
	metas := map[string]*AppMeta{
		"core": {Name: "core"},
		"a":    {Name: "a", Requires: []string{"b"}},
		"b":    {Name: "b", Requires: []string{"core"}},
	}
	in := []js.App{{Name: "core"}, {Name: "a"}, {Name: "b"}}
	out, err := orderApps(in, metas)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range out {
		names = append(names, a.Name)
	}
	if strings.Join(names, ",") != "core,b,a" {
		t.Fatalf("ordem inesperada: %v", names)
	}
	// dependência ausente
	if _, err := orderApps([]js.App{{Name: "core"}, {Name: "a"}}, metas); err == nil || !strings.Contains(err.Error(), "is not installed") {
		t.Fatalf("esperava erro de dependência ausente, veio %v", err)
	}
	// ciclo
	ciclo := map[string]*AppMeta{"x": {Name: "x", Requires: []string{"y"}}, "y": {Name: "y", Requires: []string{"x"}}}
	if _, err := orderApps([]js.App{{Name: "x"}, {Name: "y"}}, ciclo); err == nil || !strings.Contains(err.Error(), "circular") {
		t.Fatalf("esperava erro de ciclo, veio %v", err)
	}
}

func TestResolveLinkTitles(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.Run(ctx, "Administrator", func(c *Ctx) error {
		titles, err := c.LinkTitles("User", []string{"Administrator", "inexistente"})
		if err != nil {
			t.Fatalf("LinkTitles error: %v", err)
		}
		if titles["Administrator"] != "Administrator" {
			t.Fatalf("esperava 'Administrator', veio %q", titles["Administrator"])
		}
		if titles["inexistente"] != "inexistente" {
			t.Fatalf("esperava fallback para name, veio %q", titles["inexistente"])
		}
		return nil
	})
}

func TestLinkFieldSearchByTitleAndSearchFields(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.Run(ctx, "Administrator", func(c *Ctx) error {
		p, err := c.NewDoc("Pessoa", Doc{"nome": "Maria Comércio", "cpf": "999.888.777-66"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Insert(p, SaveOpts{}); err != nil {
			t.Fatal(err)
		}

		ped, err := c.NewDoc("Pedido", Doc{"cliente": "Maria Comércio"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Insert(ped, SaveOpts{}); err != nil {
			t.Fatal(err)
		}

		// 1. Busca pelo nome diretamente no campo Link cliente
		rows, err := c.GetList("Pedido", ListArgs{
			OrFilters: []any{[]any{"cliente", "like", "%Maria%"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("esperava 1 pedido buscando por 'Maria', veio %d", len(rows))
		}

		// Busca sem acento deve encontrar o título do Link com acento.
		rows, err = c.GetList("Pedido", ListArgs{
			OrFilters: []any{[]any{"cliente", "like", "%comercio%"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("esperava 1 pedido buscando por 'comercio', veio %d", len(rows))
		}

		// 2. Busca pelo CPF (searchFields de Pessoa, não o valor armazenado na coluna cliente de Pedido)
		rows, err = c.GetList("Pedido", ListArgs{
			OrFilters: []any{[]any{"cliente", "like", "%888.777%"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("esperava 1 pedido buscando pelo CPF '888.777', veio %d", len(rows))
		}

		// 3. Count com OrFilters
		count, err := c.Count("Pedido", nil, []any{[]any{"cliente", "like", "%888.777%"}})
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("esperava count 1 buscando por CPF, veio %d", count)
		}

		// 4. Busca por valor inexistente
		rows, err = c.GetList("Pedido", ListArgs{
			OrFilters: []any{[]any{"cliente", "like", "%Inexistente%"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Fatalf("esperava 0 pedidos, veio %d", len(rows))
		}

		return nil
	})
}
