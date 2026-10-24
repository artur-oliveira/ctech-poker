import 'package:intl/intl.dart';

/// Keep API identifiers out of player-facing copy. Enabling real-money entry
/// also requires wallet consent, fees and the server gate; it is not a label swap.
enum GameMode {
  chips('sandbox', 'Fichas'),
  real('real', 'Dinheiro real');

  const GameMode(this.apiValue, this.label);
  final String apiValue, label;
  String amount(num value) => this == real
      ? NumberFormat.currency(
          locale: 'pt_BR',
          symbol: 'R\$',
        ).format(value / 100)
      : NumberFormat.decimalPattern('pt_BR').format(value);
}
