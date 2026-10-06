package i18n

// english is the source catalog. Every key in the project must be defined here;
// a key missing from this catalog is a build-time error caught by
// TestEnglishIsComplete, not a runtime surprise.
var english = Catalog{
	// guide: no device in debugging mode
	//
	// Deliberately does not say "plug in your phone": by far the most common
	// real cause is a phone that is already plugged in with USB debugging off,
	// and telling someone to reconnect a cable they already connected is the
	// fastest way to lose their trust. Check the phone before the cable.
	"guide.no_device.headline": "Reflexo cannot see a phone yet",
	"guide.no_device.detail": "Reflexo only sees the device once USB debugging is " +
		"switched on and authorised. Check the steps below — look at the phone " +
		"before touching the cable.",
	"guide.no_device.step1": "On the phone, open Settings › Developer options › USB debugging and check it is on.",
	"guide.no_device.step2": "If that option is missing, open Settings › About phone and tap Build number seven times to reveal developer options.",
	"guide.no_device.step3": "Unlock the screen, then unplug and replug the cable.",
	"guide.no_device.step4": "Accept the \"Allow USB debugging?\" prompt on the phone.",
	"guide.no_device.step5": "No prompt? Try another USB port — some front-panel ports only charge.",

	// guide: neutral wait on cold launch
	"guide.starting.headline": "Getting Reflexo ready",
	"guide.starting.detail": "Checking for a connected phone. This can take a few seconds — your phone " +
		"does not need to be fully ready yet.",
	"guide.starting.step1": "Plug the phone into the computer with a data-capable cable.",
	"guide.starting.step2": "First use only: enable USB debugging under Settings on the phone.",

	// guide: adb could not run at all
	"guide.adb_unavailable.headline": "Could not run ADB",
	"guide.adb_unavailable.detail": "Reflexo could not talk to the Android Debug Bridge. " +
		"A common cause on a work computer: the Android USB driver was not installed " +
		"and Windows Update is blocked.",
	"guide.adb_unavailable.step1": "Try a different cable or port — charge-only cables are the number one cause.",
	"guide.adb_unavailable.step2": "On a company network, ask support for the ADB driver package.",
	"guide.adb_unavailable.step3": "As a workaround, mirror over Wi-Fi: enable Wireless debugging on the phone.",

	// guide: device seen but not authorised
	"guide.unauthorized.headline": "Confirm debugging on the phone",
	"guide.unauthorized.detail": "%s is connected, but it has not been authorised yet. " +
		"A prompt should be open on the phone — it sometimes closes quickly and only " +
		"comes back when you unlock the screen.",
	"guide.unauthorized.step1":        "Look at the phone screen: there should be a request called \"Allow USB debugging?\".",
	"guide.unauthorized.step2":        "Tick \"Always allow from this computer\" and tap Allow.",
	"guide.unauthorized.step3":        "If no prompt appeared, unlock the screen and replug the cable.",
	"guide.unauthorized.step4":        "Check on the phone that USB debugging is on in Developer options.",
	"guide.unauthorized.generic_name": "your phone",

	// guide: device seen but the handshake failed
	"guide.offline.headline": "Unstable connection to the phone",
	"guide.offline.detail": "The device was detected but ADB lost contact. " +
		"Common causes: another ADB program running at the same time, or the phone in " +
		"battery saver mode.",
	"guide.offline.step1": "Unplug and replug the cable.",
	"guide.offline.step2": "Close other emulators or programs that use ADB.",
	"guide.offline.step3": "On the phone, switch airplane mode on and then off.",
	"guide.offline.step4": "If it persists, restart the computer and try again.",

	// guide: pre-Android states
	"guide.pre_android.headline": "Phone in %s",
	"guide.pre_android.detail": "The device is in a state before Android starts, so there is " +
		"no screen to mirror. Restart the phone to leave this mode.",
	"guide.pre_android.step1":           "Restart the phone.",
	"guide.pre_android.step2":           "Once it has finished booting, replug the cable.",
	"guide.pre_android.step3":           "Check that USB debugging is still enabled.",
	"guide.pre_android.mode_recovery":   "recovery mode",
	"guide.pre_android.mode_bootloader": "bootloader mode",

	// guide: an adb state we do not model
	"guide.unknown.headline": "Unknown connection state",
	"guide.unknown.detail": "ADB reported the state %q, which Reflexo does not know yet. " +
		"It is either a new ADB state or a hardware-specific case.",
	"guide.unknown.step1": "Replug the cable and try again.",
	"guide.unknown.step2": "Update Reflexo to the latest version.",

	// guide: more than one device
	"guide.multiple.headline": "%d phones connected",
	"guide.multiple.detail": "Reflexo mirrors one device at a time. Unplug the others to " +
		"continue — or unplug everything and connect only the one you want.",
	"guide.multiple.step1": "Unplug the phones you will not use.",
	"guide.multiple.step2": "Wait a few seconds until only one device is left.",

	// guide: the happy path
	"guide.ready.headline":         "%s ready to mirror",
	"guide.ready.detail_known":     "USB debugging authorised (%s). You can start mirroring.",
	"guide.ready.detail_unknown":   "USB debugging authorised. You can start mirroring.",
	"guide.ready.step_start":       "Tap Start mirroring to begin.",
	"guide.ready.generic_name":     "phone",
	"guide.ready.tip_right_click":  "Right-click in the window goes back.",
	"guide.ready.tip_middle_click": "Middle-click goes to the home screen.",
	"guide.ready.tip_fullscreen":   "Alt + F toggles fullscreen.",

	// guide: vendor-specific warnings
	"guide.vendor.xiaomi.note": "Note: on Xiaomi, Redmi and POCO devices there is a second " +
		"debugging option that is often left off. Without it the mirror appears but " +
		"keyboard and mouse do not work.",
	"guide.vendor.xiaomi.step1": "On the phone: Developer options › \"USB debugging (Security settings)\" — turn it on.",
	"guide.vendor.xiaomi.step2": "Restart the phone once after enabling that option.",
	"guide.vendor.xiaomi.step3": "On some models an active SIM plan is required.",

	// shared fragments
	"shared.android": "Android %s",
	"shared.api":     "API %s",

	// setup / installation
	"setup.preparing":        "Preparing Reflexo",
	"setup.preparing_detail": "Starting…",
	"setup.failed_title":     "Could not prepare Reflexo",
	"setup.note": "This happens once. Reflexo downloads, verifies and installs scrcpy " +
		"without asking for administrator rights.",
	"setup.ready":        "scrcpy %s ready.",
	"setup.adb_starting": "scrcpy installed. ADB is still starting…",

	// severity badges. These are read by index from guide.Severity, so the
	// numbering matters; TestSeverityKeysMatchGuide keeps them aligned.
	"severity.idle":   "waiting",
	"severity.action": "do this",
	"severity.warn":   "attention",
	"severity.ready":  "ready",

	// static chrome of the web UI, applied through data-i18n attributes
	"ui.connecting":         "connecting…",
	"ui.start":              "Start mirroring",
	"ui.running":            "Mirroring…",
	"ui.close_to_stop":      "Close the mirror window to stop.",
	"ui.unavailable":        "Reflexo unavailable",
	"ui.conn_lost":          "Could not reach Reflexo. Close and reopen the program.",
	"ui.start_failed":       "could not start",
	"ui.start_conn_failed":  "connection to Reflexo failed",
	"ui.attribution_prefix": "Mirroring by",
	"ui.attribution_suffix": "— Romain Vimont, Apache 2.0.",
	"ui.non_affiliation":    "Reflexo is an independent project, not affiliated with scrcpy.",

	"guide.adb_recovering.headline": "Reflexo is reconnecting to the phone",
	"guide.adb_recovering.detail":   "The connection to the phone dropped. Reflexo is restarting it on its own — this usually takes a few seconds and your phone is fine.",
	"guide.adb_recovering.step1":    "Wait a few seconds. No need to unplug anything.",
	"ui.demo_banner":                "Demonstration — no phone is being used.",
	"ui.demo_try":                   "See how it works",
	"ui.demo_stop":                  "Stop the demonstration",

	// errors, mapped from internal failures in app.friendlyError
	"error.certificate": "Could not verify the authenticity of the internet connection. " +
		"On a company network this is usually a proxy inspecting the download.",
	"error.no_network": "No internet access. Reflexo needs to download scrcpy once.",
	"error.timeout":    "The download took too long and was interrupted. Please try again.",
	"error.integrity": "The downloaded file failed its security check and was discarded. " +
		"Try again — if it persists, contact support.",
	"error.permission": "Windows or the antivirus blocked the scrcpy installation. " +
		"Check the permissions of the install folder.",

	// unsupported platform
	"error.unsupported_platform": "Reflexo has no build for %s/%s yet. Available platforms: %s",

	// platform names, which are proper nouns and identical across languages
	"platform.windows": "Windows",
	"platform.darwin":  "macOS",
	"platform.linux":   "Linux",
}

// portugueseBrazil is the primary translation.
var portugueseBrazil = Catalog{
	// Não diz "conecte seu celular": a causa mais comum é um celular já
	// conectado com a Depuração USB desligada, e mandar alguém replugar um
	// cabo que ele acabou de plugar é o jeito mais rápido de perder a confiança
	// do usuário. Confira o celular antes do cabo.
	"guide.no_device.headline": "O Reflexo ainda não vê nenhum celular",
	"guide.no_device.detail": "O Reflexo só enxerga o aparelho quando a Depuração USB " +
		"está ligada e autorizada. Siga os passos abaixo — confira o celular antes " +
		"de mexer no cabo.",
	"guide.no_device.step1": "No celular, abra Ajustes › Opções do desenvolvedor › Depuração USB e confirme que está ligada.",
	"guide.no_device.step2": "Se a opção não existir, abra Ajustes › Sobre o telefone e toque 7 vezes em Número da build para liberá-la.",
	"guide.no_device.step3": "Desbloqueie a tela, tire o cabo e recoloque.",
	"guide.no_device.step4": "Aceite o aviso \"Permitir depuração USB?\" que aparecer no celular.",
	"guide.no_device.step5": "Sem aviso? Troque de porta USB — algumas da frente do computador só carregam.",

	// Estado neutro na inicializacao a frio
	"guide.starting.headline": "Preparando o Reflexo",
	"guide.starting.detail": "Procurando um celular conectado. Pode levar alguns segundos — " +
		"não precisa preparar o celular agora.",
	"guide.starting.step1": "Conecte o celular ao computador com um cabo de dados.",
	"guide.starting.step2": "No primeiro uso, ative a Depuração USB em Ajustes no celular.",

	"guide.adb_unavailable.headline": "Não foi possível executar o ADB",
	"guide.adb_unavailable.detail": "O Reflexo não conseguiu conversar com o Android Debug Bridge. " +
		"Causa comum em computador corporativo: o driver USB do Android não foi instalado " +
		"e o Windows Update está bloqueado.",
	"guide.adb_unavailable.step1": "Tente em outro cabo ou outra porta USB — cabos só de carga são o erro nº1.",
	"guide.adb_unavailable.step2": "Se estiver em rede da empresa, peça ao suporte o pacote do driver ADB.",
	"guide.adb_unavailable.step3": "Como alternativa, espelhe pelo Wi-Fi: ative Depuração USB sem fio no celular.",

	"guide.unauthorized.headline": "Confirme a depuração no celular",
	"guide.unauthorized.detail": "%s está conectado, mas a autorização ainda não foi dada. " +
		"Um popup deve estar aberto na tela do aparelho — às vezes ele some rápido e só " +
		"volta se você desbloquear a tela.",
	"guide.unauthorized.step1":        "Olhe a tela do celular: deve haver um pedido chamado \"Permitir depuração USB?\".",
	"guide.unauthorized.step2":        "Marque \"Sempre permitir deste computador\" e toque em Permitir.",
	"guide.unauthorized.step3":        "Se o popup não apareceu, desbloqueie a tela e reconecte o cabo.",
	"guide.unauthorized.step4":        "No celular, confira em Opções do desenvolvedor se Depuração USB está ligada.",
	"guide.unauthorized.generic_name": "seu celular",

	"guide.offline.headline": "Conexão com o celular instável",
	"guide.offline.detail": "O aparelho foi detectado mas o ADB perdeu a comunicação. " +
		"Causa comum: outro programa de ADB rodando ao mesmo tempo, ou o celular em " +
		"modo de economia de energia.",
	"guide.offline.step1": "Desconecte e reconecte o cabo.",
	"guide.offline.step2": "Feche outros emuladores ou programas que usem ADB.",
	"guide.offline.step3": "Na tela do celular, ative o modo avião e desligue-o.",
	"guide.offline.step4": "Se persistir, reinicie o computador e tente de novo.",

	"guide.pre_android.headline": "Celular em %s",
	"guide.pre_android.detail": "O aparelho está em um estado anterior ao Android, então não " +
		"existe tela para espelhar. Reinicie o celular para sair desse modo.",
	"guide.pre_android.step1":           "Reinicie o celular.",
	"guide.pre_android.step2":           "Após o boot completo, reconecte o cabo.",
	"guide.pre_android.step3":           "Confirme que a depuração USB continua ativada.",
	"guide.pre_android.mode_recovery":   "modo de recuperação",
	"guide.pre_android.mode_bootloader": "modo bootloader",

	"guide.unknown.headline": "Estado de conexão desconhecido",
	"guide.unknown.detail": "O ADB reportou o estado %q, que o Reflexo ainda não conhece. " +
		"É um estado novo do ADB ou um caso de hardware específico.",
	"guide.unknown.step1": "Reconecte o cabo e tente novamente.",
	"guide.unknown.step2": "Atualize o Reflexo para a versão mais recente.",

	"guide.multiple.headline": "%d celulares conectados",
	"guide.multiple.detail": "O Reflexo espelha um aparelho por vez. Desconecte os " +
		"demais para continuar — ou desconecte todos e conecte só o que quiser usar.",
	"guide.multiple.step1": "Desconecte do computador os celulares que não vai usar.",
	"guide.multiple.step2": "Espere alguns segundos até restar apenas um aparelho.",

	"guide.ready.headline":         "%s pronto para espelhar",
	"guide.ready.detail_known":     "Depuração USB autorizada (%s). Você pode iniciar o espelhamento.",
	"guide.ready.detail_unknown":   "Depuração USB autorizada. Você pode iniciar o espelhamento.",
	"guide.ready.step_start":       "Toque em Iniciar para espelhar.",
	"guide.ready.generic_name":     "celular",
	"guide.ready.tip_right_click":  "Botão direito na janela = voltar.",
	"guide.ready.tip_middle_click": "Botão do meio = ir para a tela inicial.",
	"guide.ready.tip_fullscreen":   "Alt + F = alternar tela cheia.",

	"guide.vendor.xiaomi.note": "Atenção: em aparelhos Xiaomi, Redmi e POCO existe uma segunda " +
		"opção de depuração que costuma ficar desligada. Sem ela o espelho aparece, mas " +
		"teclado e mouse não funcionam.",
	"guide.vendor.xiaomi.step1": "No celular: Opções do desenvolvedor › \"Depuração USB (Configurações de segurança)\" — ative.",
	"guide.vendor.xiaomi.step2": "Reinicie o celular uma vez depois de ativar essa opção.",
	"guide.vendor.xiaomi.step3": "Em alguns modelos é obrigatório ter um chip SIM com plano ativo.",

	"shared.android": "Android %s",
	"shared.api":     "API %s",

	"setup.preparing":        "Preparando o Reflexo",
	"setup.preparing_detail": "Iniciando…",
	"setup.failed_title":     "Não foi possível preparar o Reflexo",
	"setup.note": "Esta etapa acontece uma única vez. O scrcpy é baixado, conferido e " +
		"instalado sem pedir permissão de administrador.",
	"setup.ready":        "scrcpy %s pronto.",
	"setup.adb_starting": "scrcpy instalado. O ADB ainda está inicializando…",

	"severity.idle":   "aguardando",
	"severity.action": "faça isso",
	"severity.warn":   "atenção",
	"severity.ready":  "pronto",

	"ui.connecting":         "conectando…",
	"ui.start":              "Iniciar espelhamento",
	"ui.running":            "Espelhando…",
	"ui.close_to_stop":      "Feche a janela do espelho para encerrar.",
	"ui.unavailable":        "Reflexo indisponível",
	"ui.conn_lost":          "Não foi possível falar com o Reflexo. Feche e abra o programa novamente.",
	"ui.start_failed":       "não foi possível iniciar",
	"ui.start_conn_failed":  "falha de conexão com o Reflexo",
	"ui.attribution_prefix": "Espelhamento por",
	"ui.attribution_suffix": "— Romain Vimont, Apache 2.0.",
	"ui.non_affiliation":    "Reflexo é um projeto independente, sem afiliação com o scrcpy.",

	"guide.adb_recovering.headline": "Reconectando ao celular",
	"guide.adb_recovering.detail":   "A conexão com o celular caiu. O Reflexo está resolvendo sozinho — costuma levar alguns segundos, e o celular está tudo bem.",
	"guide.adb_recovering.step1":    "Espere alguns segundos. Não precisa desconectar nada.",
	"ui.demo_banner":                "Demonstração — nenhum celular está sendo usado.",
	"ui.demo_try":                   "Veja como funciona",
	"ui.demo_stop":                  "Parar a demonstração",

	"error.certificate": "Não foi possível verificar a autenticidade da conexão com a internet. " +
		"Em rede corporativa isso costuma ser um proxy que inspeciona o download.",
	"error.no_network": "Sem acesso à internet. O Reflexo precisa baixar o scrcpy uma única vez.",
	"error.timeout":    "O download demorou demais e foi interrompido. Tente novamente.",
	"error.integrity": "O arquivo baixado não passou na verificação de segurança e foi descartado. " +
		"Tente novamente — se persistir, avise o suporte.",
	"error.permission": "O Windows ou o antivírus bloqueou a instalação do scrcpy. " +
		"Verifique as permissões da pasta de instalação.",

	"error.unsupported_platform": "O Reflexo ainda não tem versão para %s/%s. Plataformas disponíveis: %s",

	// platform names, which are proper nouns and identical across languages
	"platform.windows": "Windows",
	"platform.darwin":  "macOS",
	"platform.linux":   "Linux",
}
