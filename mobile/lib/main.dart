import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'core/api.dart';
import 'core/design.dart';
import 'features/poker_logo.dart';
import 'core/preferences.dart';
import 'core/session.dart';
import 'features/home.dart';
import 'features/widgets.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
  await PokerPreferences.instance.load();
  runApp(const PokerApp());
}

class PokerApp extends StatefulWidget {
  const PokerApp({super.key});
  @override
  State<PokerApp> createState() => _PokerAppState();
}

class _PokerAppState extends State<PokerApp> {
  final session = PokerSession();
  late final api = PokerApi(session);
  bool restoring = true;
  Object? error;
  @override
  void initState() {
    super.initState();
    session.addListener(changed);
    session
        .restore()
        .catchError((Object e) {
          error = e;
        })
        .whenComplete(() {
          if (mounted) setState(() => restoring = false);
        });
  }

  void changed() {
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    session.removeListener(changed);
    session.dispose();
    api.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => MaterialApp(
    key: ValueKey(session.signedIn),
    title: 'CTech Poker',
    debugShowCheckedModeBanner: false,
    locale: const Locale('pt', 'BR'),
    supportedLocales: const [Locale('pt', 'BR')],
    localizationsDelegates: GlobalMaterialLocalizations.delegates,
    theme: PokerTheme.dark,
    home: restoring
        ? const Scaffold(body: Center(child: CircularProgressIndicator()))
        : session.signedIn
        ? PokerHome(api: api)
        : LoginScreen(session: session, initialError: error),
  );
}

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key, required this.session, this.initialError});
  final PokerSession session;
  final Object? initialError;
  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  bool busy = false;
  @override
  Widget build(BuildContext context) => Scaffold(
    body: SafeArea(
      child: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(28),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 440),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const Align(
                  alignment: Alignment.centerLeft,
                  child: PokerLogo(size: 76),
                ),
                const SizedBox(height: 28),
                Text(
                  'Sua próxima mão\ncomeça aqui.',
                  style: Theme.of(context).textTheme.displaySmall,
                ),
                const SizedBox(height: 16),
                const Text(
                  'CTech Poker\nTexas Hold’em com a sua comunidade, onde você estiver.',
                ),
                const SizedBox(height: 36),
                if (widget.initialError != null)
                  const Text(
                    'Não foi possível recuperar a sessão neste dispositivo.',
                  ),
                FilledButton.icon(
                  onPressed: busy
                      ? null
                      : () async {
                          setState(() => busy = true);
                          await safely(context, widget.session.login);
                          if (mounted) setState(() => busy = false);
                        },
                  icon: Icon(busy ? Icons.hourglass_top : Icons.login),
                  label: Text(busy ? 'Entrando…' : 'Entrar com CTech Accounts'),
                ),
                const SizedBox(height: 20),
                const Text(
                  'O login abre no navegador seguro do seu aparelho.',
                  textAlign: TextAlign.center,
                ),
              ],
            ),
          ),
        ),
      ),
    ),
  );
}
