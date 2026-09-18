import 'package:flutter/material.dart';
import 'package:flutter_svg/flutter_svg.dart';
export '../core/icons.dart';

/// Exact Lucide SVGs exported from the web client, inheriting control states.
class PokerIcon extends StatelessWidget {
  const PokerIcon(
    this.name, {
    super.key,
    this.size,
    this.color,
    this.semanticLabel,
  });
  final String name;
  final double? size;
  final Color? color;
  final String? semanticLabel;
  @override
  Widget build(BuildContext context) {
    final theme = IconTheme.of(context);
    final ink = color ?? theme.color ?? Theme.of(context).colorScheme.onSurface;
    return Center(
      widthFactor: 1,
      heightFactor: 1,
      child: SizedBox(
        width: size ?? theme.size ?? 24,
        height: size ?? theme.size ?? 24,
        child: SvgPicture.asset(
          'assets/icons/$name.svg',
          width: size ?? theme.size ?? 24,
          height: size ?? theme.size ?? 24,
          colorFilter: ColorFilter.mode(
            ink.withValues(alpha: ink.a * (theme.opacity ?? 1)),
            BlendMode.srcIn,
          ),
          semanticsLabel: semanticLabel,
          excludeFromSemantics: semanticLabel == null,
        ),
      ),
    );
  }
}
