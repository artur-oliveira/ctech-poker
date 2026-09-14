import 'package:flutter/material.dart';
import '../core/api.dart';
import 'library.dart';
import 'widgets.dart';

class CollectionsScreen extends StatelessWidget {
  const CollectionsScreen({super.key, required this.api});
  final PokerApi api;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Coleções e revisão')),
    body: SafeArea(
      child: AsyncPanel(
        load: () => api.get('/v1.0/players/me/hand-collections'),
        builder: (context, data, _) => ListView(
          padding: const EdgeInsets.all(16),
          children: [
            if (rows(data).isEmpty)
              const Text(
                'Abra uma mão e toque em Anotar e organizar para começar sua coleção.',
              ),
            for (final hand in rows(data))
              Card(
                child: ListTile(
                  leading: Icon(
                    hand['review_marked'] == true
                        ? Icons.bookmark
                        : Icons.folder_outlined,
                  ),
                  title: Text((hand['collections'] as List? ?? []).join(' · ')),
                  subtitle: Text(hand['hand_id']),
                  onTap: () => safely(context, () async {
                    final detail = await api.get(
                      '/v1.0/players/me/hand/${segment(hand['hand_id'])}',
                      query: {'mode': 'sandbox'},
                    );
                    if (context.mounted) {
                      Navigator.push(
                        context,
                        MaterialPageRoute<void>(
                          builder: (_) => HandScreen(api: api, hand: detail),
                        ),
                      );
                    }
                  }),
                ),
              ),
          ],
        ),
      ),
    ),
  );
}

class HandNotesEditor extends StatefulWidget {
  const HandNotesEditor({super.key, required this.api, required this.handId});
  final PokerApi api;
  final String handId;
  @override
  State<HandNotesEditor> createState() => _HandNotesEditorState();
}

class _HandNotesEditorState extends State<HandNotesEditor> {
  final notes = {
    for (final stage in ['preflop', 'flop', 'turn', 'river', 'showdown'])
      stage: TextEditingController(),
  };
  final collections = TextEditingController();
  bool review = false, busy = false;
  bool loaded = false;
  @override
  void dispose() {
    for (final note in notes.values) {
      note.dispose();
    }
    collections.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Anotar e organizar')),
    body: SafeArea(
      child: AsyncPanel(
        load: () => widget.api.get(
          '/v1.0/players/me/hands/${segment(widget.handId)}/meta',
        ),
        builder: (context, data, _) {
          if (!loaded) {
            for (final e in notes.entries) {
              e.value.text = data['street_notes']?[e.key] ?? '';
            }
            collections.text = (data['collections'] as List? ?? []).join(', ');
            review = data['review_marked'] == true;
            loaded = true;
          }
          return ListView(
            padding: const EdgeInsets.all(20),
            children: [
              for (final e in notes.entries)
                Padding(
                  padding: const EdgeInsets.only(bottom: 16),
                  child: TextField(
                    controller: e.value,
                    maxLines: 3,
                    maxLength: 2000,
                    decoration: InputDecoration(
                      labelText: {
                        'preflop': 'Pré-flop',
                        'flop': 'Flop',
                        'turn': 'Turn',
                        'river': 'River',
                        'showdown': 'Showdown',
                      }[e.key],
                    ),
                  ),
                ),
              TextField(
                controller: collections,
                decoration: const InputDecoration(
                  labelText: 'Coleções',
                  helperText: 'Separe os nomes por vírgula.',
                ),
              ),
              SwitchListTile(
                title: const Text('Marcar para revisão'),
                value: review,
                onChanged: (value) => setState(() => review = value),
              ),
              FilledButton(
                onPressed: busy
                    ? null
                    : () => safely(context, () async {
                        setState(() => busy = true);
                        try {
                          await widget.api.request(
                            '/v1.0/players/me/hands/${segment(widget.handId)}/meta',
                            method: 'PUT',
                            body: {
                              'street_notes': {
                                for (final e in notes.entries)
                                  e.key: e.value.text.trim(),
                              },
                              'review_marked': review,
                              'collections': collections.text
                                  .split(',')
                                  .map((s) => s.trim())
                                  .where((s) => s.isNotEmpty)
                                  .toSet()
                                  .toList(),
                            },
                          );
                          if (context.mounted) Navigator.pop(context);
                        } finally {
                          if (mounted) setState(() => busy = false);
                        }
                      }),
                child: const Text('Salvar anotações'),
              ),
            ],
          );
        },
      ),
    ),
  );
}
