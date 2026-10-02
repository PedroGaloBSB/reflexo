# Reflexo

Espelha a tela do seu celular Android no computador, **sem complicação e sem
pedir permissão de administrador**.

Você conecta o cabo USB e o Reflexo descobre o que fazer, te explica na tela e
inicia o espelhamento.

> O espelhamento é feito pelo **[scrcpy](https://github.com/Genymobile/scrcpy)**,
> projeto de Romain Vimont e colaboradores, sob Apache License 2.0. O Reflexo é
> um projeto independente, sem afiliação com o scrcpy.

---

## O problema que ele resolve

O [scrcpy](https://github.com/Genymobile/scrcpy) é excelente — leve, rápido,
com baixa latência. Mas é **100% linha de comando**. Para quem usa o computador
profissionalmente, isso é uma barreira real:

```console
C:\Users\joao> scrcpy --turn-screen-off
ERROR: Could not find any ADB device
```

O usuário não sabe se o cabo está ruim, se faltou ativar a depuração USB, se
precisa aceitar um popup no celular, ou se a máquina precisa de driver. Ele só
vê um erro e uma tela que fecha sozinha.

O Reflexo é a camada que falta: **detecta o estado real e diz o próximo passo.**

## Como funciona

```
conecta o cabo
      │
      ▼
┌─────────────────────┐
│  adb devices -l     │
└─────────────────────┘
      │
      ├── (vazio) ────────► "Ative a Depuração USB e reconecte o cabo"
      ├── unauthorized ───► "Aceite 'Permitir depuração USB?' no celular"
      ├── offline ────────► "Reinicie o celular e reconecte o cabo"
      └── device ─────────► adb shell getprop  →  "Samsung · Galaxy A55 · Android 16"
                                    │
                                    ▼
                            [  Iniciar  ]
                                    │
                                    ▼
                            janela do espelho
```

Não é um manual fixo: são **instruções para o estado atual**. O mesmo aparelho
em estado diferente recebe uma mensagem diferente.

Quando não há celular conectado, o Reflexo mostra uma **demonstração** do fluxo
completo — é o modo `--demo`, ou o botão "Veja como funciona" na tela.

## Recursos

- **Zero instalação.** Um arquivo executável. Pode rodar de um pen drive.
- **Sem administrador.** Não instala nada: sem instalador, serviço, registro ou
  driver próprio. (Há uma exceção do próprio Windows, veja
  [Solução de problemas](#solução-de-problemas).)
- **Windows, Linux e macOS.** Mesmo código, binários separados.
- **Detecção automática.** SO, fabricante, modelo e versão do Android.
- **Instruções contextuais.** Adaptadas ao que está acontecendo agora.
- **Demonstração embutida.** Mostra o produto funcionando sem precisar de celular.
- **Registro de diagnóstico.** Cada mudança de estado fica registrada num log
  local, para quando o TI precisar ver o que aconteceu.

## Requisitos

- Celular com Android 5.0 (API 21) ou superior
- Cabo USB
- Depuração USB ativada

O `adb` **é baixado automaticamente** junto com o scrcpy no primeiro uso. Você não
precisa instalar o Android SDK, nem o scrcpy, nem nada.

## Instalação

1. Baixe o arquivo do seu sistema na página de releases.
2. Extraia e execute.

Não há instalador — é só um binário.

> **Sobre o Windows.** O arquivo não é assinado, então o SmartScreen pode mostrar
> "O Windows protegeu seu computador" na primeira execução. Clique em
> **Mais informações** → **Executar assim mesmo**. Isso é comum em projetos
> abertos e não publicados numa loja.

> **Em ambientes corporativos**, o projeto foi desenhado para rodar sem privilégio
> administrativo. Ainda assim, políticas de AppLocker/WDAC podem bloquear
> qualquer executável não assinado — nesse caso, é decisão do TI da rede e não
> do aplicativo.

### Verificando o download

Cada release publica um `SHA256SUMS.txt` com o hash de cada binário. Confira
antes de executar — é a compensação por o binário não ser assinado.

```console
# Windows (PowerShell)
certutil -hashfile reflexo-windows-amd64.exe SHA256

# macOS
shasum -a 256 reflexo-macos-arm64

# Linux
sha256sum reflexo-linux-amd64
```

Compare o resultado com a linha correspondente no `SHA256SUMS.txt` da release.
Se os valores forem iguais, o arquivo chegou íntegro.

## Solução de problemas

### O Windows quer instalar um driver USB na primeira conexão

Na maioria dos Windows 10 e 11 **não acontece nada**: o driver que o `adb` usa
(WinUSB) já vem no sistema, e o Reflexo não instala nenhum driver por conta
própria.

Mas em alguns casos o próprio Windows oferece instalar um driver USB ao
reconhecer o celular — aparelho com identificação fora do padrão, ou máquina
corporativa com o Windows Update bloqueado. Se isso ocorrer, **pode ser
necessário privilégio de administrador nessa etapa específica**. É
comportamento do Windows ao reconhecer o dispositivo, não uma exigência do
Reflexo.

Se você passar por isso, abra uma issue com a versão do Windows e o modelo do
aparelho. Esse comportamento não foi validado em laboratório controlado — é um
risco conhecido, documentado para ser reavaliado com relatos reais.

### O Reflexo não vê o celular

1. **Depuração USB ativada?** Em Ajustes › Opções do desenvolvedor.
2. **Autorizou o computador?** Com o cabo conectado, o celular pergunta
   "Permitir depuração USB?" — é preciso tocar em Permitir.
3. **Cabo de dados.** Cabo só de carregamento não conecta.
4. **Tente outra porta USB**, diretamente no computador (não em hub).

O Reflexo mostra o estado em que o celular está e o que fazer — se a mensagem
não fizer sentido, o log em `%LOCALAPPDATA%\reflexo\logs\reflexo.log` tem o
detalhe técnico.

## Stack técnica

Escrito em **Go**, resulta em binário único por plataforma, sem runtime externo.
A interface é HTML embutido aberto no navegador padrão do sistema — escolha
deliberada para não depender de WebView2 nem de runtimes que possam estar
ausentes em máquina corporativa.

Sem dependências de terceiros. O `go.mod` não tem nenhuma biblioteca externa.

## Estrutura

```
reflexo/
├── main.go              ponto de entrada
├── internal/
│   ├── app/             estado, polled de adb, servidor local
│   ├── catalog/         versão e plataformas do scrcpy
│   ├── device/          parsing da saída do adb
│   ├── diag/            mensagens de falha sem console
│   ├── fetch/           download e verificação por SHA-256
│   ├── guide/           regra estado → instrução
│   ├── i18n/            todo texto visível, pt-BR e en
│   └── logfile/         registro local rotativo
├── web/                 HTML/CSS/JS embutidos no binário
└── THIRD_PARTY.md       licenças e o que é redistribuído
```

O scrcpy **não** está neste repositório nem dentro do binário. Ele é baixado em
tempo de execução do release oficial do Genymobile e verificado por SHA-256.
Ver `AGENTS.md` §3 e `THIRD_PARTY.md`.

## Licença

Apache License 2.0 — ver [`LICENSE`](LICENSE).

O binário do Reflexo **não contém nenhum software de terceiros**. O scrcpy é
baixado do upstream pelo usuário, na máquina dele, e conferido contra o
manifesto de checksums publicado pelo próprio Genymobile. O registro completo
está em [`THIRD_PARTY.md`](THIRD_PARTY.md).

### Agradecimento e atribuição

O espelhamento, a codificação de vídeo, a baixa latência e toda a engenharia por
trás que tornam este produto possível vêm do
**[scrcpy](https://github.com/Genymobile/scrcpy)**, criado por **Romain Vimont /
@rom1v** e colaboradores, sob Apache License 2.0.

O Reflexo é uma ferramenta independente que **usa** o scrcpy. Não é afiliado,
endossado ou aprovado pelo projeto scrcpy. A marca "scrcpy" pertence aos seus
detentores e é usada aqui apenas para descrever compatibilidade.

## Estrutura de decisão

As regras que governam este projeto — proibições de marca, restrição de
zero-administrador, stack e processo — estão em [`AGENTS.md`](AGENTS.md).
Qualquer contribuição deve passar por lá antes de ser aceita.