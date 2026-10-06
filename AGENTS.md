# AGENTS.md — Regras do Projeto Reflexo

> Este arquivo é a fonte da verdade. Qualquer humano, dev ou IA que trabalhe
> neste repositório **deve** ler antes de escrever qualquer linha de código.
>
> Se uma instrução aqui conflitar com um pedido pontual, a instrução deste
> arquivo vence — e a regra precisa ser alterada aqui primeiro.

---

## 1. Identidade

- **Nome do produto:** Reflexo
- **Nome do executável:** `reflexo` (`reflexo.exe` no Windows)
- **Origem do nome:** "reflexo" (imagem refletida) + "reflexos" (reação
  rápida / baixa latência). Ver `README.md` para o raciocínio completo.

## 2. Proibições absolutas (marca)

O scrcpy é distribuído sob Apache License 2.0, cuja seção 6 **não concede
licença de marca**. Portanto:

- ❌ É PROIBIDO usar "scrcpy", "scrcpy-launcher", "scrcpy-helper", "scrcpyX"
  ou qualquer variação no **nome** do produto, binário, pacote ou domínio.
- ❌ É PROIBIDO usar os logos/ícones do scrcpy (`app/data/scrcpy.png`,
  `scrcpy.svg`, `disconnected.png`).
- ❌ É PROIBIDO sugerir afiliação, parceria ou endosso com o projeto scrcpy.
- ✅ É OBRIGATÓRIO atribuir o scrcpy publicamente, na tela "Sobre" e no
  `README`, como software de terceiros sob Apache 2.0.

Consequência prática: a string `scrcpy` pode existir **apenas** em contextos
descritivos (ex.: "espelhamento compatível com scrcpy 4.1"), nunca como marca
própria.

## 3. Arquitetura inegociável

O Reflexo **NÃO** é um fork do scrcpy. É um *wrapper* (orquestrador).

- O scrcpy é consumido como **binário pré-compilado**, baixado em tempo de
  instalação a partir dos releases oficiais do Genymobile.
- O scrcpy **nunca** é copiado, forkado ou vendorizado neste repositório.
- Atualizar o scrcpy = trocar a versão fixada em `internal/catalog.go`.
  **Não** deve exigir alteração de código fora do catálogo.
- Não há código C, C++ ou Java neste projeto.

Consequência: o Reflexo herda a Apache 2.0 do scrcpy apenas por
redistribuição de binário. Ver `README.md` § Licença.

## 4. Restrição: zero privilégio de administrador

Requisito originado em ambiente corporativo. É **hard requirement**, não
opcional.

- Nada de instalador, serviço, driver, entrada de registro ou pasta em
  `Program Files`.
- Nada de UAC / elevação.
- Deve rodar de diretório portável (incluindo pen drive).
- Nada que dependa de rede além do escopo estritamente necessário, para não
  quebrar em rede corporativa com proxy bloqueado.
- Não exigir WebView2, .NET Desktop Runtime ou Python no PATH do usuário.

**Teste de aceite obrigatório:** deslogar, usar uma conta sem privilégios
administrativos, executar a partir de `C:\Users\<usuário>\AppData\Local\...`.
Se quebrar, é bug — não "limitação do ambiente".

## 5. Stack

- **Go** para o binário. Um binário por plataforma, sem runtime externo.
- Alvos, limitados ao que o scrcpy upstream **realmente publica**:
  - `windows/amd64` e `windows/386`
  - `linux/amd64`
  - `darwin/amd64` e `darwin/arm64`

- ⚠️ **`linux/arm64` não existe e não deve ser anunciado.** O scrcpy v4.1
  publica apenas `scrcpy-linux-x86_64`. Não há binário aarch64 para Linux, nem
  para celular Android (que também é arm64 — é preciso binário para o *host*).
  Suportar isso exigiria compilar o scrcpy, proibido por §3. Se surgir demanda,
  isso vira uma ADR nova, não uma linha de catálogo.
- Nenhuma biblioteca de GUI nativa obrigatória.
- UI renderizada como HTML embutido via `embed`, aberto no navegador padrão.
  Escolha registrada em ADR-0002.

## 6. Escopo funcional (v1)

Fluxo único, sem menus, sem configurações avançadas:

1. Detectar plataforma e SO.
2. Detectar dispositivo via `adb devices -l`.
3. Ler propriedades do aparelho via `adb shell getprop`.
4. Exibir instruções **contextuais ao estado atual**, não um manual fixo.
5. Lançar o scrcpy com argumentos default sensatos.

Estados que a UI precisa cobrir obrigatoriamente:

| Estado `adb` | Instrução exibida |
|---|---|
| (vazio) | Conecte o cabo USB |
| `unauthorized` | Aceite "Permitir depuração USB?" no aparelho |
| `offline` | Autorize a depuração USB em Opções do desenvolvedor |
| `device` | Detectado: fabirante · modelo · versão Android → Iniciar |

Requisito especial: **aparelhos Xiaomi/Redmi** devem ser detectados e
alertar sobre `USB debugging (Security settings)`, que é a causa nº1 da
mensagem `INJECT_EVENTS permission`. Ver `doc/xiaomi.md`.

## 7. Regras de compatibilidade de versão

- O binário do scrcpy e o `scrcpy-server` **precisam** ser o mesmo par de
  versão. Nunca misturar.
- Toda atualização do scrcpy exige re-teste manual no aparelho de referência.
- Versões de SO Android suportadas espelham as do scrcpy (mín. API 21).

## 8. Licenças de terceiros

Todo binário redistribuído no instalador precisa de entrada em
`THIRD_PARTY.md`, com: nome, versão, licença, URL e quem fez o download.

- scrcpy — Apache 2.0
- Android platform-tools (adb) — Apache 2.0
- FFmpeg — **LGPL** (o build oficial do scrcpy não usa `--enable-gpl`)
- SDL3 — zlib
- libusb — LGPL 2.1+

Nenhuma dependência GPL deve ser introduzida sem revisão explícita.

## 9. Antes de commitar

- [ ] `go build ./...` passa
- [ ] `go vet ./...` passa
- [ ] `gofmt -l .` retorna vazio
- [ ] `go test ./...` passa
- [ ] Testado **sem** privilégio de administrador
- [ ] Se tocou em `web/` ou `internal/i18n`: **a página foi aberta num navegador**
      e conferida nos dois idiomas (ver ARCH-0002)
- [ ] Nenhuma ocorrência proibida da marca fora de contexto descritivo
      (`grep -ri scrcpo --include=* -l` revisado)
- [ ] `THIRD_PARTY.md` atualizado se alguma dependência mudou
- [ ] Se tocou em `internal/app/demo.go`: a demonstração foi vista numa página
      de verdade (`reflexo.exe --demo`), não só no teste

## 10. Decisões

| ID | Assunto | Status |
|---|---|---|
| ADR-0001 | Binário único Go vs. instalador | ✅ aceito — binário único |
| ADR-0002 | UI em navegador vs. WebView nativo | ✅ aceito — navegador padrão |
| ADR-0003 | Canal de distribuição | ✅ aceito — repositório e Releases públicos no GitHub |
| ADR-0004 | Quantidade de aparelhos simultâneos | ✅ aceito — um por vez |
| ADR-0005 | Assinatura de código no Windows | aceito - nao assinar (v1) |
| ADR-0006 | Idioma da interface | ✅ aceito — pt-BR e en, catálogo central |
| ADR-0007 | Detectar celular conectado com depuração desligada | ⏳ implementado — Windows; macOS/Linux em aberto |
| ADR-0008 | Modo demonstração sem celular | ✅ aceito — v1.2 |
| ADR-0009 | Binário sem janela de console | ✅ aceito — reverte decisão anterior |
| ADR-0010 | Onde o payload do scrcpy fica | aceito - nunca em pasta sincronizada |
| ADR-0011 | Depuracao USB como pre-requisito ineliminavel | aceito - reconhecido, documentado e guiado |

### ADR-0003 — Distribuição pública

Repositório público no GitHub, binários nos Releases, sem loja.

Consequências que **precisam** ser resolvidas antes do primeiro release:

- **ADR-0005 (bloqueante).** Um `.exe` não assinado dispara o SmartScreen do
  Windows. Para usuário leigo isso aparece como "O Windows protegeu seu
  computador", que é exatamente o oposto do produto que estamos vendendo.
  Opções a avaliar: certificado EV (~$300–400/ano), certificado OV com
  contagem de reputação, ou aceitar e documentar. **Decisão do dono do
  projeto, não do time técnico.**
- macOS exigirá notarização para rodar sem avisos em versões recentes.

### ADR-0004 — Um aparelho por vez

Reflexo espelha **um** aparelho. Se houver mais de um conectado, a UI
instrui a desconectar os demais. Isso mantém a interface em um estado apenas
e evita a ambiguidade de "qual celular eu tô controlando agora?".

Consequência: a função `guide.Of` recebe a lista completa e escolhe o
comportamento pelo `len`. Não existe seleção de dispositivo na v1.

### ADR-0005 - Assinatura de código no Windows

Decisao: **nao assinar**. O binário do Reflexo nao tera assinatura
Authenticode, e a notarizacao do macOS fica para depois.

Tres motivos, na ordem em que apareceram:

1. **O mercado mudou.** O argumento historico do certificado EV - reputacao
   imediata no SmartScreen - foi corroido pela Microsoft a partir de 2021; ate EV
   comeca sem reputacao. O CA/Browser Forum passou a exigir token de hardware
   para certificado de assinatura de código. Pagar R$300-400/ano para isso nao
   compra o que comprava.
2. **Assinatura nao resolve o bloqueio do público-alvo.** SmartScreen e mensagem
   de consumidor. O público do Reflexo é máquina corporativa com AppLocker/WDAC,
   onde o bloqueio vem de politica de rede - e politica de rede nao muda porque
   o binário tem assinatura. O certificado so ajudaria se o TI da empresa
   colocasse aquele publisher na lista.
3. **O custo de nao assinar e um clique.** "Mais informacoes" -> "Executar assim
   mesmo". Todo projeto aberto que nao publica numa loja passa por isso.

Mitigacao de baixo custo: cada release publica um `SHA256SUMS.txt` dos binários,
para o usuario conferir integridade antes de executar. O README ja mostra o
aviso do SmartScreen **antes** de o usuario deparar com ele.

Pendencia separada: a notarizacao do macOS exige conta de desenvolvedor Apple
(US$99/ano) e e obrigatoria em versoes recentes. Tema proprio, nao decidido.
### ADR-0006 — Idioma da interface

Todo texto visível ao usuário vive em `internal/i18n/catalogs.go`, com
inglês como fonte e pt-BR como tradução. O navegador **não tem strings
próprias**: recebe o texto já traduzido no payload da API e o aplica nos
elementos `data-i18n` de `web/index.html`.

O idioma é escolhido uma vez, na inicialização, a partir de `REFLEXO_LANG`
e das variáveis POSIX. Um produto que troca de idioma no meio da leitura é
pior do que um que escolheu o idioma errado de primeira.

Consequências e armadilhas já encontradas:

- A chave de catálogo de uma severidade vem do servidor (`State.SeverityKey`).
  O número da severidade existe só para a cor do CSS. O navegador **não sabe**
  que `0` significa "aguardando" — quando tentamos montar a chave lá, a UI
  exibiu `SEVERITY.0` cru para o usuário. Está em
  `TestWebLayerNeverBuildsKeysFromNumbers`.
- `guide.SeverityCount` é um sentinela `iota`. Inserir uma severidade acima
  dele incrementa a contagem e faz o teste de rótulos falhar, o que é
  intencional.
- Chaves que ninguém usa acumulam tradução morta. `TestUIStringsCoverMarkup`
  cobra os dois sentidos.

### ADR-0007 — "Depuração USB desligada" é indistinguível de "sem celular"

`adb devices` devolve lista vazia em duas situações muito diferentes: nenhum
celular conectado, e um celular **conectado** com a Depuração USB desligada.
Na segunda, o aparelho não anuncia a interface ADB e se comporta como
pendrive — foi exatamente o caso do primeiro aparelho real testado
(SM-A556E, Windows: `MI_00` MTP e `MI_01` serial presentes, `MI_03` ADB
ausente).

Na v1 não diferenciamos os dois casos. A consequência é que a instrução
**não pode afirmar** que o cabo está fora, senão manda o usuário replugar um
cabo que ele acabou de plugar. Por isso o texto de "nenhum aparelho" começa
pelo celular e só depois menciona o cabo, e isso está protegido por
`TestNoDeviceChecksPhoneBeforeCable`.

Decisão, implementada: o Reflexo **enuncia** a diferença quando o sistema
prova, e **se cala** quando não prova. Onde não há prova confiável, o texto
antigo continua — que é a escolha honesta, não uma falta.

Onde a prova é possível hoje — Windows, sem privilégio e sem subprocesso:

| Sinal | Onde se lê | O que significa |
|---|---|---|
| `Class_ff&SubClass_42&Prot_01` | `SPDRP_COMPATID` de cada interface presente | A interface ADB está no ar: o celular está bem e o problema é o adb |
| `Class_06&SubClass_01&Prot_01` **e** VID de fabricante Android | idem | Há celular na mesa com a depuração desligada |
| nada dos dois | — | nada de Android no barramento |

Os dois marcadores foram medidos nesta máquina, em dois aparelhos de
fabricantes diferentes: um Samsung SM-A556E (VID_04e8) e um Redmi Pad 2
(VID_2717). E o detalhe que quase passou: o Samsung registra
`Class_FF&SubClass_42` e a Xiaomi registra `Class_ff&SubClass_42`. A comparação
é insensível a caixa por causa disso, e
`TestMarkerMatchingIsCaseInsensitive` existe para manter isso verdadeiro — é a
forma que esse bug costuma ter: funciona no aparelho do desenvolvedor.

Duas tentativas anteriores estão registradas em `usb_windows.go` porque cada
uma parece correta e não é:

1. **Enumerar a interface ADB pela classe do SetupAPI não devolve nada útil.**
   O Windows registra-a na classe genérica `USBDevice` {88bae032-…}; "ADB
   Interface" é apenas um nome amigável que o driver instala, não uma classe.
2. **Subir até o dispositivo pai para perguntar se o compósito está completo
   falha aqui por um motivo que não tem nada a ver com USB.** A instância pai
   reporta `Present: False` enquanto as próprias filhas reportam
   `Present: True`, numa máquina saudável com celular plugado. O que for isso,
   não serve para sustentar uma mensagem.

Uma terceira armadilha, descoberta ao implementar: a árvore USB do Windows
guarda **todo aparelho que a máquina já viu**. O Redmi Note 9S, plugado há
meses, ainda está lá com a lista inteira de interfaces. `DIGCF_PRESENT` não é
otimização, é o que faz o detector funcionar — sem essa flag o fantasma basta
para o Reflexo anunciar um celular que não está na mesa.

A lista de fabricantes é **whitelist, e a direção da falha é a própria
decisão**. Fabricante fora da lista faz o Reflexo voltar ao "nenhum celular" de
antes, que é um erro cosmético. Fabricante dentro por engano faria o Reflexo
chamar a câmera digital do usuário de celular e mandar alguém procurar uma
opção de depuração que não existe. Subafirmar incomoda; sobreafirmar faz o
usuário desconfiar da tela. MTP não é invenção do Android — câmeras,
players e e-readers também falam MTP, e é por isso que o marcador sozinho não
basta.

Custo real, medido: **~11ms por varredura** com 121 dispositivos presentes. Só
roda quando o adb respondeu e não viu nada, ou seja, enquanto o usuário está
ocioso esperando um celular. **Não há cache**: cache seria estado a invalidar
para uma economia que a medição não pediu.

O probe é consultado em exatamente um lugar, `a.debuggingOff()`, no caminho em
que o adb está saudável e a lista saiu vazia. Nos outros, quem tem a resposta é
a máquina de saúde do adb, e acusar o celular ali seria palpite.
`TestTheProbeIsNeverAskedWhenItCouldNotBeTrusted` trava os três pontos.

Ainda não implementado: macOS e Linux, que continuam devolvendo `UsbUnknown` e
mantêm o texto antigo. A receita completa, com os números de classe que
valem, está em `usb_other.go`. Celular Android de fabricante fora da lista
também cai no texto antigo — por construção, não por acidente.
### ADR-0008 — Modo demonstração

Quase ninguém que abre o Reflexo tem um celular Android plugado. O que essa
pessoa vê é "O Reflexo ainda não vê nenhum celular": um produto que se apresenta
pelo que **não** tem não vai despertar interesse em ninguém.

Quando não há aparelho, o Reflexo mostra o que teria mostrado.

**A regra que mantém isso honesto:** a demonstração alimenta o mesmo
`guide.Of()` que alimenta um aparelho real, e renderiza pela mesma página. Não
existe uma tela de demonstração separada. Uma demo construída com a cópia
própria do texto concordaria com o produto até a primeira vez que qualquer um
dos dois fosse editado, e depois anunciaria algo que o Reflexo não faz — o que
é pior do que não ter demo. `TestDemoDescribesFakePhoneThroughTheRealGuide`
cobra headline, detalhe, severidade e passos idênticos aos do guia real.

As demais regras vieram de casos concretos:

- **Aparelho real vence o roteiro.** `gather()` encerra a demonstração assim que
  o adb reporta um aparelho. Quem começou a demo e depois conectou o celular
  quer o celular; um laço que ignora isso é a pior impressão que o produto pode
  deixar.
- **A demo se anuncia.** `State.Demo` força a faixa âmbar. Sem ela, quem assiste
  tem todas as razões para achar que o produto está falando do próprio celular.
- **Nada é executado.** Não há aparelho para espelhar; chamar o scrcpy sem
  aparelho ou pisca um erro ou se anexa ao aparelho que estiver plugado.
  `TestStartDuringDemoLaunchesNothing`.
- **O aparelho fictício nunca vai para o log.** O valor inteiro do arquivo de
  suporte é poder ser acreditado.
- **O botão só aparece quando não há nada a mostrar** (`State.CanDemo`), e
  serve também de saída — `POST /api/demo` alterna. Quem disparou a demo sem
  querer nunca fica preso num laço que não consegue encerrar.
- **O laço não tem fim.** Um roteiro que para numa tela é uma captura de tela.
- **A demo existe antes da instalação**, porque quem precisa dela é, por
  definição, quem ainda não instalou nada
  (`TestWatchDoesNotWaitForInstallWhenDemoing`).

Duas entradas: a flag `--demo` para mandar um link, e o botão na tela para
descoberta. A flag é testada na linha de comando; o botão é o que faz o
visitante parar.

O celular fictício é um Pixel 7 de propósito. Samsung e Xiaomi fazem o guia
emitir instrução específica de fabricante, e quem não tem nenhuma das duas leria
aquela instrução como sendo sobre o próprio celular.

### ADR-0009 — Binário sem janela de console (reverte decisão anterior)

A janela de console foi mantida na v1 como botão de emergência. Ela não é um
botão de emergência: é um gerador de relatório de acidente. Fechá-la envia
CTRL_CLOSE_EVENT, o Go reporta SIGTERM, o Reflexo encerra — e a página no
navegador fica aberta, congelada em silêncio, sem nenhuma indicação do que
aconteceu. Ocorreu numa sessão real; o log dizia `encerrando: terminated`.

E todo usuário fecha a janela de console em algum momento, porque uma caixa
preta com cursor é a definição de programa que se deve fechar. A interação mais
provável era justamente a que matava o processo.

Decisão: `-H windowsgui` no `build.ps1`. As falhas chegam ao usuário por
`internal/diag` (MessageBoxW no Windows), não por stderr.

`TestReleaseBinaryHasNoConsoleWindow` lê o subsistema PE do binário construído e
exige 2 (GUI). `TestConsoleSubsystemIsDetectable` é o controle negativo: o mesmo
parser precisa relatar 3 (console) sem a flag. Um guard que não pode falhar não
é guard.

Consequência: não há mais nada legível no console. É o ponto — mas significa que
o log passou a ser o único canal de diagnóstico, e uma falha que aconteça antes
do log abrir precisa aparecer como diálogo.

### ADR-0010 — Onde o payload do scrcpy fica

As primeiras versões gravavam o payload em `os.UserConfigDir()`, que no Windows
é `%AppData%` — a pasta **Roaming**. O log já estava em LOCALAPPDATA por causa
disso; o payload, de 25 MB, ficou no lugar errado.

Roaming é replicado pelo OneDrive e por perfil de domínio em máquina
corporativa, que é o público do produto. O payload ali significaria: sincronização
lenta no primeiro uso, consumo de cota, e em rede limitada uma espera longa
antes de qualquer coisa aparecer.

A regra agora, por plataforma:

| Plataforma | Diretório | Por quê |
|---|---|---|
| Windows | `LOCALAPPDATA` (`os.UserCacheDir`) | Nunca sincronizado, nunca exigiu elevação |
| macOS | `~/Library/Application Support` | Não replicado por padrão; `~/Library/Caches` seria esvaziado pelo sistema e forçaria um novo download |
| Linux | `$XDG_DATA_HOME`, senão `~/.local/share` | O spec XDG põe dados de aplicação ali; `~/.config` é para configuração editável |

A migração é cortesia, nunca requisito: `MigratePayload` move o que encontrou e,
se falhar, o Reflexo baixa de novo. Baixar de novo custa 25 MB uma vez; recusar
a iniciar custaria o produto inteiro.

O que a migração **não** move: nada que não comece com `scrcpy-`. O nome é o
critério inteiro — `fetch.Ensure` extrai o archive upstream, cujo diretório de
topo é sempre `scrcpy-<plataforma>-<versão>`. Uma pasta que o usuário colocou ali
manualmente nunca é movida, renomeada ou apagada.

`TestScrcpyIsAlwaysFetchedFromUpstream` continua sendo a guarda da conclusão de
LGPL: enquanto o download vier do upstream, não há redistribuição. Este ADR
muda **onde** o payload descansa, não **de onde** ele vem.

### ADR-0011 - Depuração USB e Opções do desenvolvedor no aparelho são requisitos de produto

Decisão: o Reflexo **assume**, documenta e **orienta** — mas **nunca tenta
eliminar** — o pré-requisito de ter Depuração USB ativada no celular.

Motivos:

1. **Não é limitação do scrcpy, é do Android.** O `adbd` só entra em operação
   quando a Depuração USB está marcada, e o scrcpy é um cliente ADB. Qualquer
   fluxo que espelhe pela mesma via falha na mesma parede.
2. **É modelo de segurança, não inconveniência.** Depuração USB concede
   controle equivalente a root. Uma forma de espelhar sem ela seria uma falha
   de segurança no próprio sistema.
3. **`scrcpy --otg` não espelha.** Injeta toque/teclado via HID sem depuração,
   mas **não captura a tela**. Para a promessa do Reflexo (ver o aparelho no
   computador), `--otg` não é alternativa.
4. **As exceções reais são produtos outros** (Phone Link, DeX, Miracast), com
   contas e hardware próprios — não cabem em um wrapper de scrcpy.

Consequência para o produto: o que o Reflexo faz é **domar** o requisito —
explicar, reduzir os passos ao que é realmente necessário no estado atual e
nunca fingir que ele não existe. Quando o celular está conectado mas sem a
interface ADB, o caminho é orientar a ativação da depuração, não inventar um
atilho bypass.
### ARCH-0001 — Checks-then-act em código com concorrência

Nenhuma rotina que "lê uma condição e age em cima dela" pode usar dois locks
separados. `App.Start` lia `Running` sob `RLock` e gravava depois sob `Lock`:
doze requisições simultâneas passaram todas na guarda e lançaram quatro
espelhos do mesmo celular. Agora `startMu` segura a operação inteira,
inclusive o `exec`.

Nenhum teste sequencial pega isso. `TestConcurrentStartLaunchesOnce` usa uma
barreira e `WaitGroup` para sobrepor de verdade as chamadas.

### ARCH-0002 — Detector de texto do usuário no web/

`TestNoUserFacingStringsInWeb` reprova literais de português ou inglês em
`web/app.js`. O `index.html` tem uma exceção deliberada: alguns literais em
pt-BR existem para a página ser legível nos poucos milissegundos antes do
primeiro estado, e todos eles carregam `data-i18n`.

Vale a pena saber que esse teste **não** substitui abrir a página num
navegador. Três defeitos da migração de i18n só apareceram no DOM renderizado:
a chave de severidade, o rodapé com "by ... by ...", e o `applyStatic` que
sobrescrevia a linha de plataforma com "conectando…".

### ARCH-0003 — Log, diagnóstico e build

`internal/logfile` é o único canal de diagnóstico. As regras saíram de falhas,
não de planejamento:

- **LOCALAPPDATA, nunca Roaming.** Roaming é sincronizado na nuvem (OneDrive,
  known-folder-move) em máquina corporativa — o público-alvo — e o log contém
  números de série de aparelho. `TestLogDirIsNotInRoaming`.
- **Transição de estado, não cada consulta.** A 2s, um log por consulta são
  1800 linhas idênticas por hora.
- **A rotação acontece com o arquivo fechado.** O Windows recusa renomear um
  arquivo aberto; rotacionar no lugar matava o log exatamente quando ele ficava
  grande.
- **`Close()` é terminal.** Uma escrita depois do `Close` reabria o arquivo e
  deixaria um handle aberto depois do encerramento.
- **Nil-safe e thread-safe**, para que registrar um log nunca seja a razão de o
  Reflexo quebrar.

`internal/diag` é o canal de falha para quando não há console: MessageBoxW no
Windows, `erro.txt` entregue ao desktop no Linux/macOS. Sem dependência de GUI
no caminho de falha — é justamente ali que não se pode depender de nada.

`build.ps1` é a única forma de produzir binário de release, e
`main_windows_test.go` cobra o subsistema PE, então a flag não pode ser removida
sem quebrar a suíte.

### ARCH-0004 - Pipeline de release

As releases sao publicadas por `.github/workflows/release.yml`, disparada por
tag `v*.*.*`. Tres jobs:

1. **test** no `windows-latest` — roda a suíte onde o teste de subsistema PE
   (`main_windows_test.go`) consegue executar. Sem esse job a garantia de
   "binário sem janela de console" deixaria de ser coberta.
2. **build** no `ubuntu-latest` — cross-compila os 5 alvos suportados. O Windows aparece duas vezes na matriz (amd64 e 386), mas linux/arm64 fica de fora de propósito: o upstream não publica essa build, então um binário para ali compilaria e seria publicado só para falhar em runtime. O `-H windowsgui` é **condicional**: com `GOOS=linux` essa
   flag faz o linker produzir um PE (MZ) em vez de um ELF. O build passa e o
   artefato está errado — foi verificado. Por isso a flag só é aplicada no job
   Windows.
3. **release** — baixa os artefatos, gera o `SHA256SUMS.txt` com `sha256sum` e
   publica tudo como assets da release.

O `SHA256SUMS.txt` é gerado aqui, nunca à mão. Um checksum escrito manualmente
é o passo que alguém esquece, e aí a garantia que o ADR-0005 promete para
compensar a falta de assinatura de código simplesmente não existe.

`build.ps1` continua existindo para o build local de desenvolvimento. O
workflow é o caminho de release, não o de desenvolvimento.

O workflow **não pode ser testado localmente** — só empurrando para o GitHub.
O primeiro release é o teste. O que dá para verificar antes é a matriz de build
e o comando de checksum, e ambos foram verificados.

**Validação feita:** a sintaxe do YAML foi verificada com PyYAML (Python
disponível no ambiente), e a estrutura está correta — 3 jobs, `needs` encadeado,
matriz com 5 entradas, `permissions: contents: write`.

**Risco residual explícito:** `actionlint` e `act` **não estão instalados** neste
ambiente. A validação com PyYAML cobre a sintaxe e a estrutura, mas **não** cobre
referências de ações (`actions/checkout@v4`, `softprops/action-gh-release@v2`),
sintaxe de expressões `${{ }}`, nem o comportamento real do `upload-artifact` /
`download-artifact` / `gh-release`. Essas são as partes que só o GitHub
confirma. O erro mais comum nesse tipo de pipeline — permissão de upload de
asset da release — **não é visível em teste local nenhum**.

### ARCH-0005 - Saude do adb e recuperacao automatica

Sessao real perdida em 2026-10-05, registrada no log:

```
10:40:21  aparelho: dq7ppfnjr84tjnuw estado=device
10:40:23  espelhamento iniciado
10:46:15  espelhamento terminou com erro: exit status 2
10:46:20  ERROR  adb devices falhou: * daemon not running
10:46:20  instrucao: [warn] Nao foi possivel executar o ADB
```

O celular nunca recusou nada e nunca perdeu autorizacao. O servidor do adb
morreu, o scrcpy perdeu o transporte e saiu com codigo 2. A partir dai todo
poll falhava, e o Reflexo mostrava "ADB indisponivel" para sempre, sem tentar
consertar nada. **Reiniciar o Reflexo era a unica solucao, e nada na interface
dizia isso.**

O que foi feito (`internal/device/health.go`, `internal/app/health.go`):

- **Classificacao da falha.** `daemon not running` e diferente de um timeout e
  diferente de um handshake falhando. Cada texto foi lido de uma falha real, nao
  de documentacao.
- **Recuperacao nao destrutiva.** `FailureKind.NeedsRestart()` retorna sempre
  `false`, e isso e decisao. O servidor em tcp:5037 e um singleton da maquina,
  compartilhado com Android Studio, VS Code e qualquer ferramenta de fabricante.
  Derruba-lo para tratar o nosso sintoma quebraria programas que o usuario nao
  pediu para tocarmos - o oposto do AGENTS.md §4. Medido: um daemon travado
  responde `adb start-server` com "could not read ok from ADB Server", e insistir
  em loop faz a porta nunca assentar.
- **Limiar de duas falhas.** Um poll ruim e ruido: celular desconectado, uma
  transacao USB repetindo, uma hesitacao. Duas seguidas sao um padrao.
- **Backoff 5s/10s/20s/40s/60s.** Recupero a cada 2s seria trinta reinicios por
  minuto.
- **Estado proprio na interface.** `guide.adb_recovering.*` diz "Reconectando ao
  celular" durante a recuperacao. A mensagem antiga mandava o usuario reinstalar
  driver que estava instalado.
- **Lista vazia suspeita.** Logo apos a queda do espelho, `adb devices` pode sair
  com codigo zero e imprimir nada — saida byte-a-byte igual a de uma mesa sem
  celular. `suspiciousEmpty()` confere o adb de novo antes de aceitar.

**Cuidado:** a recuperacao nunca pode afirmar sucesso que nao aconteceu. Quando
`Recover` falha, o estado e o de fim de linha, nao o de "reconectando".

### RESIDUO CONHECIDO - daemon do adb lento nesta maquina

Medido em 2026-10-05, e **nao e defeito do Reflexo**:

- `adb start-server` retorna em ~1,5s com `protocol fault: connection reset`
  e exit nao-zero, mesmo com a porta 5037 livre.
- O processo do daemon sobe, mas leva **~8s** para efetivamente escutar a 5037.
- Durante essa janela, qualquer cliente recebe `connection reset`.
- `netstat` chegou a mostrar a 5037 em `LISTENING` com o cliente ja tendo
  desistido, e o `adb` seguinte ainda falhou — o handshake depende de mais que o
  bind.

Consequencia pratica: num start a frio desta maquina, o Reflexo ve duas falhas,
tenta recuperar, e o `Recover` falha porque o daemon ainda esta subindo. A
recuperacao esta correta; o ambiente e lento demais para ela.

O que falta decidir, e e escolha de produto:

1. **Aumentar o tempo de espera do primeiro poll**, dando ao daemon tempo de
   subir antes de declarar falha.
2. **Nao tentar recuperar nos primeiros N segundos** apos o inicio, porque o
   delay e esperado e nao e sintoma.
3. Aceitar e documentar, ja que `adb devices` no primeiro poll subsequente
   funciona.

Nenhuma das tres foi implementada. Medir em outras maquinas antes de escolher:
isto pode ser especifico deste hardware.
