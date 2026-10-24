import 'package:flutter/material.dart';

/// Rasterized directly from ui/public/svgs/logo.svg at 512px (no redraw).
class PokerLogo extends StatelessWidget {
  const PokerLogo({super.key, this.size = 38});
  final double size;

  @override
  Widget build(BuildContext context) => Image.asset(
    'assets/brand/logo.png',
    width: size,
    height: size,
    fit: BoxFit.contain,
    filterQuality: FilterQuality.high,
    semanticLabel: 'CTech Poker',
  );
}
