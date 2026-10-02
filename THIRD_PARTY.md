# THIRD_PARTY.md

Registro de todo software de terceiros que o Reflexo **usa**, e do que ele
**distribui** junto dos seus artefatos de release.

Atualizar este arquivo é item obrigatório da lista de verificação em
`AGENTS.md` §9.

---

## O que o Reflexo distribui

**Nenhum software de terceiros.** O artefato de release é `reflexo.exe`, um
binário Go puro com a interface web embutida. Ele não contém o scrcpy, nem o
adb, nem o FFmpeg, nem qualquer biblioteca nativa.

Isso não é detalhe de empacotamento — é o que sustenta a seção seguinte. O
teste `TestScrcpyIsAlwaysFetchedFromUpstream` fixa o host de download em
`github.com/Genymobile/scrcpy`: o dia em que o Reflexo passar a servir uma
cópia do scrcpy do próprio servidor, a redistribuição deixa de ser zero e a
obrigação da LGPL descrita abaixo volta a ser nossa.

---

## Baixados em tempo de execução

O Reflexo **baixa** o scrcpy do release oficial do Genymobile no primeiro uso,
verifica o SHA-256 contra o `SHA256SUMS.txt` publicado pelo próprio upstream na
mesma release, e só então extrai. Cada usuário recebe a cópia diretamente do
Genymobile; o Reflexo não intermedia nem retém a redistribuição.

### scrcpy

| Campo | Valor |
|---|---|
| Versão | 4.1 (fixada em `internal/catalog/catalog.go`) |
| Licença | Apache License 2.0 |
| Origem | https://github.com/Genymobile/scrcpy |
| Autoria | Romain Vimont (`@rom1v`) e colaboradores |
| Download | Release oficial do Genymobile, verificado por SHA-256 |
| Binários | `scrcpy-win64`, `scrcpy-win32`, `scrcpy-linux-x86_64`, `scrcpy-macos-x86_64`, `scrcpy-macos-aarch64` |

O scrcpy **não** é forkado, vendorizado, modificado nem redistribuído. É
baixado como artefato binário íntegro e verificado.

O `LICENSE.txt` do scrcpy acompanha o payload baixado, porque o Reflexo o
copia junto na extração.

Exigências de atribuição da Apache 2.0 cumpridas mesmo sem redistribuição,
porque o Reflexo exibe o scrcpy na interface:

- O rodapé da página credita publicamente o scrcpy e o autor
- O `README.md` do Reflexo atribui o projeto
- O Reflexo declara não-afiliação de forma explícita

### Android SDK Platform-Tools (adb)

| Campo | Valor |
|---|---|
| Versão | empacotada junto com o scrcpy do release oficial |
| Licença | Apache License 2.0 |
| Origem | https://developer.android.com/studio/releases/platform-tools |

### Dependências nativas do scrcpy

Não são compiladas nem modificadas por nós — entram como parte dos binários
baixados do upstream.

| Biblioteca | Licença | Papel |
|---|---|---|
| FFmpeg | **LGPL** (build oficial não usa `--enable-gpl`) | Decodificação de vídeo e áudio |
| SDL3 | zlib | Janela, entrada, threads |
| libusb | LGPL 2.1+ | HID/OTG |

### A questão do FFmpeg e da LGPL

As builds oficiais de Linux e macOS do scrcpy são compiladas com
`-Dstatic=true`, e a LGPL exige que **quem distribui** uma obra linkada
estaticamente ofereça a possibilidade de relink, ou publique os fontes
correspondentes.

A obrigação é de quem distribui aquele binário — neste caso, o Genymobile. O
Reflexo não o distribui: não o contém, não o empacota, não o serve. Ele baixa
do upstream, na máquina do usuário final, e verifica a integridade contra o
manifesto do próprio Genymobile. Cada cópia entregue o é pelo próprio detentor
dos direitos.

Portanto **não há fonte de FFmpeg a publicar no release do Reflexo**, e esta
seção não bloqueia a distribuição pública.

> Esta é uma leitura de engenharia, não parecer jurídico. A conclusão depende
> inteiramente da premissa "redistribuição = zero", e essa premissa está
> verificada por teste. Qualquer mudança no modelo de entrega — um build
> portátil offline, um espelho, um instalador que embuta o scrcpy — reabre a
> questão e precisa ser tratada de novo.

---

## Ferramentas de desenvolvimento (não distribuídas)

| Ferramenta | Licença | Uso |
|---|---|---|
| Go 1.27.1 | BSD-3-Clause | Compilação |
| `gofmt` / `go vet` | BSD-3-Clause | Qualidade |
| `golang.org/x/sys` | BSD-3-Clause | *(se adicionado)* chamadas de sistema |

Nenhuma dependência com licença **GPL** deve ser introduzida sem revisão
explícita registrada nesta seção.

---

## Dependências de runtime

O Reflexo **não** exige que o usuário instale nada. Não utiliza:

- WebView2 (Edge) — evita dependência que pode estar ausente em máquina corporativa
- .NET Desktop Runtime
- Python
- Java

Esta ausência é um requisito, não uma economia. Ver `AGENTS.md` §4.