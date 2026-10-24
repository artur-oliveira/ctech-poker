import 'dart:async';
import 'package:flutter/foundation.dart';

class ReplayController extends ChangeNotifier {
  int index = 0, length = 0;
  double speed = 1;
  bool playing = false;
  Timer? _timer;
  void configure(int count) {
    pause();
    length = count;
    index = 0;
    notifyListeners();
  }

  void seek(int value) {
    _timer?.cancel();
    playing = false;
    index = value.clamp(0, length > 0 ? length - 1 : 0);
    notifyListeners();
  }

  void pause() {
    _timer?.cancel();
    if (playing) {
      playing = false;
      notifyListeners();
    }
  }

  void toggle() {
    if (playing) {
      pause();
      return;
    }
    if (length < 2) return;
    if (index == length - 1) index = 0;
    playing = true;
    notifyListeners();
    _schedule();
  }

  void cycleSpeed() {
    speed = speed == 1
        ? 2
        : speed == 2
        ? .5
        : 1;
    if (playing) _schedule();
    notifyListeners();
  }

  void _schedule() {
    _timer?.cancel();
    _timer = Timer(Duration(milliseconds: (1000 / speed).round()), () {
      if (!playing) return;
      index++;
      if (index >= length - 1) {
        index = length - 1;
        playing = false;
      }
      notifyListeners();
      if (playing) _schedule();
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }
}
