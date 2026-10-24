import 'poker_icon.dart';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'package:image/image.dart' as img;
import 'package:image_picker/image_picker.dart';
import 'package:speech_to_text/speech_to_text.dart';
import 'package:webview_flutter/webview_flutter.dart';
import '../core/api.dart';
import '../core/realtime.dart';
import '../core/preferences.dart';
import '../generated/poker.pb.dart';
import 'widgets.dart';

Uint8List avatarJpeg(Uint8List bytes) {
  final decoded = img.decodeImage(bytes);
  if (decoded == null) throw const FormatException('Imagem não suportada.');
  final image = img.bakeOrientation(decoded);
  final side = image.width < image.height ? image.width : image.height;
  final cropped = img.copyCrop(
    image,
    x: (image.width - side) ~/ 2,
    y: (image.height - side) ~/ 2,
    width: side,
    height: side,
  );
  return img.encodeJpg(
    img.copyResize(cropped, width: 192, height: 192),
    quality: 85,
  );
}

Future<void> updateAvatar(PokerApi api) async {
  final photo = await ImagePicker().pickImage(
    source: ImageSource.gallery,
    maxHeight: 1024,
    maxWidth: 1024,
  );
  if (photo == null) return;
  final bytes = await compute(avatarJpeg, await photo.readAsBytes());
  final upload = await api.post('/v1.0/players/me/avatar/upload-url');
  final uri = Uri.parse(upload['url']);
  if (uri.scheme != 'https') throw StateError('Endereço de upload inválido.');
  final request = http.MultipartRequest('POST', uri)
    ..fields.addAll(Map<String, String>.from(upload['fields']))
    ..files.add(
      http.MultipartFile.fromBytes('file', bytes, filename: 'avatar.jpg'),
    );
  final response = await request.send().timeout(const Duration(seconds: 20));
  await response.stream.drain<void>();
  if (response.statusCode >= 400) {
    throw StateError('Não foi possível enviar a foto.');
  }
  await api.post('/v1.0/players/me/avatar/confirm', {
    'version': upload['version'],
  });
}

class NativePreferences extends StatelessWidget {
  const NativePreferences({super.key});
  @override
  Widget build(BuildContext context) {
    final prefs = PokerPreferences.instance;
    return Scaffold(
      appBar: AppBar(title: const Text('Preferências da mesa')),
      body: SafeArea(
        child: ListenableBuilder(
          listenable: prefs,
          builder: (context, _) => ListView(
            children: [
              SwitchListTile(
                title: const Text('Sons da mesa'),
                value: prefs.sound,
                onChanged: (value) => safely(context, () async {
                  prefs.sound = value;
                  await prefs.save();
                }),
              ),
              SwitchListTile(
                title: const Text('Narração do dealer'),
                value: prefs.narration,
                onChanged: (value) => safely(context, () async {
                  prefs.narration = value;
                  await prefs.save();
                }),
              ),
              SwitchListTile(
                title: const Text('Treino de equidade · Fichas'),
                value: prefs.trainer,
                onChanged: (value) => safely(context, () async {
                  prefs.trainer = value;
                  await prefs.save();
                }),
              ),
              ListTile(
                title: const Text('Lembrete de tempo de sessão'),
                trailing: DropdownButton<int>(
                  value: prefs.reminderMinutes,
                  items: [
                    for (final n in [0, 30, 60, 90, 120])
                      DropdownMenuItem(
                        value: n,
                        child: Text(n == 0 ? 'Desligado' : '$n min'),
                      ),
                  ],
                  onChanged: (value) => safely(context, () async {
                    prefs.reminderMinutes = value ?? 60;
                    await prefs.save();
                  }),
                ),
              ),
              const Padding(
                padding: EdgeInsets.all(16),
                child: Text(
                  'O microfone só é ativado quando você toca em Comando de voz na mesa. Toda ação reconhecida exige confirmação.',
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class VoiceCommandSheet extends StatefulWidget {
  const VoiceCommandSheet({super.key, required this.realtime});
  final PokerRealtime realtime;
  @override
  State<VoiceCommandSheet> createState() => _VoiceCommandSheetState();
}

class _VoiceCommandSheetState extends State<VoiceCommandSheet> {
  final speech = SpeechToText();
  String words = '';
  bool listening = false;
  Object? error;
  @override
  void dispose() {
    speech.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => SafeArea(
    child: Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Text('Comando de voz', style: TextStyle(fontSize: 24)),
          const Text('Diga “passar”, “pagar” ou “desistir”.'),
          const SizedBox(height: 16),
          Text(words),
          if (error != null)
            const Text(
              'Microfone indisponível. Verifique a permissão do aparelho.',
            ),
          FilledButton.icon(
            onPressed: listening
                ? null
                : () => safely(context, () async {
                    try {
                      final available = await speech.initialize();
                      if (!available) {
                        throw StateError('Microfone indisponível.');
                      }
                      if (!mounted) return;
                      setState(() {
                        listening = true;
                        error = null;
                      });
                      final version = widget.realtime.snapshot?.snapshotVersion;
                      await speech.listen(
                        listenOptions: SpeechListenOptions(localeId: 'pt_BR'),
                        onResult: (result) async {
                          if (!mounted) return;
                          setState(() => words = result.recognizedWords);
                          if (!result.finalResult) return;
                          setState(() => listening = false);
                          final action = {
                            'passar': 'check',
                            'pagar': 'call',
                            'desistir': 'fold',
                          }[words.toLowerCase().trim()];
                          if (action == null) return;
                          if (await confirm(
                                context,
                                'Confirmar comando',
                                'Você disse “$words”. Executar esta ação?',
                              ) &&
                              version ==
                                  widget.realtime.snapshot?.snapshotVersion) {
                            widget.realtime.act(action);
                          }
                        },
                      );
                    } catch (e) {
                      if (mounted) {
                        setState(() {
                          error = e;
                          listening = false;
                        });
                      }
                    }
                  }),
            icon: const PokerIcon(PokerIcons.mic),
            label: Text(listening ? 'Ouvindo…' : 'Ativar microfone'),
          ),
        ],
      ),
    ),
  );
}

class BotChallengeScreen extends StatefulWidget {
  const BotChallengeScreen({super.key, required this.realtime});
  final PokerRealtime realtime;
  @override
  State<BotChallengeScreen> createState() => _BotChallengeScreenState();
}

class _BotChallengeScreenState extends State<BotChallengeScreen> {
  static const siteKey = String.fromEnvironment('TURNSTILE_SITE_KEY');
  late final controller = siteKey.isEmpty
      ? null
      : (WebViewController()
          ..setJavaScriptMode(JavaScriptMode.unrestricted)
          ..addJavaScriptChannel(
            'PokerChallenge',
            onMessageReceived: (message) {
              if (message.message.isNotEmpty && message.message.length < 8192) {
                widget.realtime.command(
                  ClientMessage(
                    type: 'bot_challenge',
                    turnstileToken: message.message,
                  ),
                );
                if (mounted) Navigator.pop(context);
              }
            },
          )
          ..loadHtmlString(
            '''<!doctype html><html lang="pt-BR"><meta name="viewport" content="width=device-width,initial-scale=1"><body style="background:#112129;color:white;font:18px sans-serif;padding:20px"><p>Confirme que você é uma pessoa para continuar.</p><div id="challenge"></div><script>function ready(){turnstile.render('#challenge',{sitekey:${jsonEncode(siteKey)},callback:function(token){PokerChallenge.postMessage(token)}})}</script><script src="https://challenges.cloudflare.com/turnstile/v0/api.js?onload=ready&render=explicit" async defer></script></body></html>''',
            baseUrl: 'https://poker.aoctech.app/',
          ));
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Verificação de segurança')),
    body: SafeArea(
      child: siteKey.isEmpty
          ? const Padding(
              padding: EdgeInsets.all(24),
              child: Text(
                'A verificação de segurança não está configurada nesta versão do aplicativo.',
              ),
            )
          : WebViewWidget(controller: controller!),
    ),
  );
}
