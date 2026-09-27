"""
An original score for the launch video, synthesized from scratch (no
samples, no licenses): a cinematic pad, a pulse that drives the live scenes,
risers into each title card and an impact when it lands.

    python3 music.py cues.json out.wav

cues.json: {"length": seconds, "cues": [{"id": "intro", "start": 0.0, "kind": "card" | "live"}, ...]}
(build.mjs writes it from the assembled video, so every hit lands on a cut).
"""
import json
import sys

import numpy as np
from scipy.signal import butter, fftconvolve, sosfilt

SR = 48000
BPM = 100
BEAT = 60 / BPM
rng = np.random.default_rng(7)

# A minor: Am - F - C - G, one chord per bar (MIDI notes).
PROGRESSION = [
    [57, 60, 64, 71],  # Am(add9-ish voicing: A C E B)
    [53, 57, 60, 67],  # Fmaj(add9): F A C G
    [48, 55, 60, 64],  # C: C G C E
    [55, 59, 62, 69],  # G(add9): G B D A
]
ROOTS = [45, 41, 48, 43]  # A2 F2 C3 G2


def hz(midi):
    return 440.0 * 2 ** ((midi - 69) / 12)


def lowpass(x, cutoff, order=2):
    return sosfilt(butter(order, cutoff, "low", fs=SR, output="sos"), x)


def highpass(x, cutoff, order=2):
    return sosfilt(butter(order, cutoff, "high", fs=SR, output="sos"), x)


def saw(freq, n, detune=0.0, harmonics=14):
    """A band-limited sawtooth (additive), optionally detuned in cents."""
    t = np.arange(n) / SR
    f = freq * 2 ** (detune / 1200)
    out = np.zeros(n)
    for k in range(1, harmonics + 1):
        if f * k > SR / 2.2:
            break
        out += np.sin(2 * np.pi * f * k * t + rng.uniform(0, 2 * np.pi)) / k
    return out


def env(n, attack, release, curve=3.0):
    """Attack/release envelope over n samples (seconds)."""
    e = np.ones(n)
    a = min(n, int(attack * SR))
    r = min(n - a, int(release * SR))
    if a:
        e[:a] = np.linspace(0, 1, a)
    if r:
        e[n - r:] = np.linspace(1, 0, r) ** curve
    return e


def decay(n, seconds):
    return np.exp(-np.arange(n) / (seconds * SR))


class Track:
    def __init__(self, length):
        self.n = int(length * SR)
        self.dry = np.zeros((2, self.n))
        self.wet = np.zeros((2, self.n))  # sent to the reverb

    def add(self, start, sig, gain=1.0, pan=0.0, send=0.0):
        i = int(start * SR)
        if i >= self.n:
            return
        sig = sig[: self.n - i] * gain
        l, r = np.sqrt((1 - pan) / 2), np.sqrt((1 + pan) / 2)
        for bus, amt in ((self.dry, 1.0), (self.wet, send)):
            if amt:
                bus[0, i:i + len(sig)] += sig * l * amt
                bus[1, i:i + len(sig)] += sig * r * amt


# --- Instruments ------------------------------------------------------------

def pad(chord, seconds, bright=900):
    n = int(seconds * SR)
    voices = sum(saw(hz(m), n, d) for m in chord for d in (-9, 0, 8)) / (len(chord) * 3)
    return lowpass(voices, bright, 2) * env(n, 0.9, 1.2, 1.5)


def kick():
    n = int(0.45 * SR)
    t = np.arange(n) / SR
    f = 44 + 90 * np.exp(-t * 28)
    body = np.sin(2 * np.pi * np.cumsum(f) / SR) * decay(n, 0.16)
    click = highpass(rng.standard_normal(n), 2500) * decay(n, 0.004) * 0.25
    return np.tanh(1.6 * (body + click))


def hat(open_=False):
    n = int((0.22 if open_ else 0.06) * SR)
    return highpass(rng.standard_normal(n), 7500, 4) * decay(n, 0.06 if open_ else 0.012)


def bass(midi, seconds):
    n = int(seconds * SR)
    t = np.arange(n) / SR
    tone = np.sin(2 * np.pi * hz(midi) * t) + 0.35 * lowpass(saw(hz(midi), n, harmonics=6), 500)
    return tone * env(n, 0.005, seconds * 0.7, 2.0)


def pluck(midi, seconds=0.5):
    n = int(seconds * SR)
    tone = saw(hz(midi), n, harmonics=10) * 0.6 + saw(hz(midi), n, 7, harmonics=10) * 0.4
    return lowpass(tone, 2600, 2) * decay(n, 0.14) * env(n, 0.003, 0.05)


def riser(seconds):
    """Filtered noise sweeping up into a hit."""
    n = int(seconds * SR)
    noise = rng.standard_normal(n)
    out = np.zeros(n)
    blocks = 48
    for b in range(blocks):
        s, e = b * n // blocks, (b + 1) * n // blocks
        cut = 300 * (12000 / 300) ** (b / blocks)
        out[s:e] = lowpass(noise[max(0, s - 2048):e], cut)[-(e - s):]
    return out * np.linspace(0, 1, n) ** 2.2


def impact(seconds=3.2):
    n = int(seconds * SR)
    t = np.arange(n) / SR
    sub = np.sin(2 * np.pi * np.cumsum(30 + 50 * np.exp(-t * 6)) / SR) * decay(n, 0.9)
    crash = lowpass(rng.standard_normal(n), 5000) * decay(n, 0.5) * 0.35
    return np.tanh(1.4 * (sub + crash))


def reverb(x, seconds=2.8):
    n = int(seconds * SR)
    ir = rng.standard_normal((2, n)) * decay(n, seconds / 5)
    ir = np.stack([lowpass(ch, 6500) for ch in ir])
    ir /= np.abs(ir).sum(axis=1, keepdims=True) ** 0.5 * 12
    return np.stack([fftconvolve(x[c], ir[c])[: x.shape[1]] for c in range(2)])


# --- Arrangement ------------------------------------------------------------

def score(length, cues):
    tr = Track(length)
    sections = [(c, cues[i + 1]["start"] if i + 1 < len(cues) else length) for i, c in enumerate(cues)]
    live_seen = 0

    for c, end in sections:
        start, kind, cid = c["start"], c["kind"], c["id"]
        dur = end - start

        if cid == "intro":
            tr.add(0, pad(PROGRESSION[0], end + 1.2, 700), 0.55, send=0.6)
            tr.add(0.2, pluck(81, 1.6), 0.10, 0.3, send=0.8)
            continue

        if kind == "card" and cid != "arch":
            # Rise into the card, land on it, hold a chord under the words.
            tr.add(start - 1.6, riser(1.6), 0.22, send=0.3)
            tr.add(start, impact(), 1.2 if cid != "outro" else 1.35, send=0.5)
            chord = PROGRESSION[0] if cid == "outro" else PROGRESSION[3]
            tr.add(start, pad(chord, dur + (3.0 if cid == "outro" else 0.9), 1300), 0.5, send=0.7)
            if cid == "outro":
                tr.add(start, bass(33, 4.0), 0.55)
                for k, m in enumerate([69, 72, 76, 81]):
                    tr.add(start + 0.25 * k, pluck(m, 1.2), 0.12, pan=(-0.5 + k / 3), send=0.9)
            continue

        if cid == "arch":
            # Breakdown: no drums; half-time bass, pad and a slow arpeggio.
            tr.add(start - 1.0, riser(1.0), 0.12, send=0.3)
            bars = int(np.ceil(dur / (4 * BEAT)))
            for b in range(bars):
                t0 = start + b * 4 * BEAT
                chord = PROGRESSION[b % 4]
                tr.add(t0, pad(chord, 4 * BEAT + 0.8, 1600), 0.45, send=0.7)
                tr.add(t0, bass(ROOTS[b % 4] - 12, 4 * BEAT * 0.9), 0.5)
                for s in range(8):
                    m = chord[[0, 2, 1, 3, 2, 1, 3, 2][s]] + 12
                    tr.add(t0 + s * BEAT / 2, pluck(m, 0.6), 0.10, pan=0.4 * np.sin(s), send=0.7)
            continue

        # A live scene: drums back in on the one; more layers each scene.
        live_seen += 1
        steps = int(dur / (BEAT / 4))
        for s in range(steps):
            t = start + s * BEAT / 4
            bar, beat16 = divmod(s, 16)
            chord, root = PROGRESSION[bar % 4], ROOTS[bar % 4]
            if beat16 % 4 == 0:
                tr.add(t, kick(), 0.55)
            if beat16 % 2 == 0:  # 8th-note bass, pumping away from the kick
                tr.add(t, bass(root - (0 if beat16 % 4 else 12), BEAT / 2), 0.2 if beat16 % 4 else 0.14)
            if live_seen >= 2 and beat16 % 4 == 2:
                tr.add(t, hat(), 0.16, pan=0.25)
            if live_seen >= 2 and beat16 in (6, 14):
                tr.add(t, hat(True), 0.08, pan=-0.3, send=0.2)
            if live_seen >= 3 and beat16 % 2 == 1:
                m = chord[[0, 1, 2, 3, 2, 1, 3, 2][(beat16 // 2) % 8]] + 12
                tr.add(t, pluck(m, 0.35), 0.075, pan=0.5 * np.sin(s * 1.3), send=0.5)
            if beat16 == 0:
                tr.add(t, pad(chord, 4 * BEAT + 0.6, 800 + 250 * live_seen), 0.32, send=0.6)

    mix = tr.dry + reverb(tr.wet) * 0.9
    # Gentle master: soft clip, fade in/out.
    mix = np.tanh(mix * 1.1)
    fade_in, fade_out = int(0.4 * SR), int(2.2 * SR)
    mix[:, :fade_in] *= np.linspace(0, 1, fade_in)
    mix[:, -fade_out:] *= np.linspace(1, 0, fade_out) ** 1.5
    return mix / np.abs(mix).max() * 0.89


def write_wav(path, stereo):
    import wave

    pcm = (np.clip(stereo.T, -1, 1) * 32767).astype("<i2")
    with wave.open(path, "wb") as w:
        w.setnchannels(2)
        w.setsampwidth(2)
        w.setframerate(SR)
        w.writeframes(pcm.tobytes())


if __name__ == "__main__":
    spec = json.load(open(sys.argv[1]))
    write_wav(sys.argv[2], score(spec["length"], spec["cues"]))
