"""Makes the soundtrack for the 20-second video: background music plus sound effects timed to the
animation in scenes.html. Everything is synthesised here, so there is no copyright to worry about.

    python3 docs/marketing/video/sound.py /tmp/bosho-sound.wav

Music: 0-7 s slow and a little tense (the waiting), 7-18 s upbeat (the solution), 18-20 s a bright
final chord. Effects: pops for bubbles and cards, a ticking clock, a scan sweep and beep, a tap,
a ding for the added dish, a kitchen bell, rising notes for the sales chart, a chime on the logo.
"""
import sys
import wave

import numpy as np

RATE = 48000
LENGTH = 20.0
N = int(RATE * LENGTH)
rng = np.random.default_rng(7)  # fixed seed: the same sound on every run


def t_of(dur):
    return np.arange(int(RATE * dur)) / RATE


def env(n, attack=0.005, release=0.1):
    """Linear attack, exponential decay over n samples."""
    t = np.arange(n) / RATE
    a = np.clip(t / max(attack, 1e-4), 0, 1)
    return a * np.exp(-t / max(release, 1e-4))


def note(freq):
    return 440.0 * 2 ** ((freq - 69) / 12)  # MIDI number to Hz


def tone(freq, dur, release=0.25, attack=0.005, harmonics=(1, .35, .15)):
    t = t_of(dur)
    s = sum(h * np.sin(2 * np.pi * freq * (i + 1) * t) for i, h in enumerate(harmonics))
    return s * env(len(t), attack, release)


def noise(dur, release=0.05):
    n = int(RATE * dur)
    return rng.standard_normal(n) * env(n, 0.001, release)


def sweep(f0, f1, dur, release=None):
    t = t_of(dur)
    f = np.linspace(f0, f1, len(t))
    phase = 2 * np.pi * np.cumsum(f) / RATE
    e = np.sin(np.pi * np.arange(len(t)) / len(t)) if release is None else env(len(t), 0.005, release)
    return np.sin(phase) * e


def lowpass(x, alpha):
    """One-pole low-pass filter (alpha near 0 = darker)."""
    y = np.empty_like(x)
    acc = 0.0
    for i, v in enumerate(x):
        acc += alpha * (v - acc)
        y[i] = acc
    return y


left = np.zeros(N)
right = np.zeros(N)


def put(sig, at, gain=1.0, pan=0.0):
    """Mix sig into the track at time `at` seconds; pan -1 left .. +1 right."""
    i = int(at * RATE)
    sig = sig[: max(0, N - i)]
    lg, rg = gain * min(1, 1 - pan), gain * min(1, 1 + pan)
    left[i:i + len(sig)] += sig * lg
    right[i:i + len(sig)] += sig * rg


# ---------------- music ----------------
# Part A, 0-7 s: a soft minor pad (A minor) and a slow, plodding bass: waiting.
pad_t = t_of(7.0)
pad = sum(np.sin(2 * np.pi * note(m) * pad_t) for m in (57, 60, 64)) / 3
pad *= np.minimum(1, pad_t / 0.8) * np.minimum(1, (7.0 - pad_t) / 0.5)
put(pad, 0, 0.10)
for k in range(9):  # bass on every 0.75 s
    put(tone(note(45 if k % 2 == 0 else 44), 0.6, release=0.25, harmonics=(1, .5)), 0.1 + k * 0.75, 0.16)

# Part B, 7-18 s: upbeat, 112 bpm, C - G - Am - F, plucked arpeggio, bass, light drums.
beat = 60 / 112
start = 7.0
chords = [(48, (60, 64, 67, 72)), (43, (59, 62, 67, 71)), (45, (57, 60, 64, 69)), (41, (57, 60, 65, 69))]
bars = 0
t0 = start
while t0 < 18.0 - 1e-6:
    root, notes = chords[bars % 4]
    for b in range(4):  # 4 beats a bar
        tb = t0 + b * beat
        if tb >= 18.0:
            break
        put(tone(note(root), beat * 0.95, release=0.18, harmonics=(1, .6, .2)), tb, 0.20)  # bass
        kick = np.sin(2 * np.pi * np.cumsum(np.linspace(140, 45, int(RATE * .18))) / RATE) * env(int(RATE * .18), .001, .06)
        put(kick, tb, 0.30)                                                           # kick
        put(noise(0.04, 0.012), tb + beat / 2, 0.035, pan=0.3)                         # hi-hat
        for e in range(2):  # eighth-note arpeggio
            te = tb + e * beat / 2
            if te < 18.0:
                m = notes[(b * 2 + e) % 4]
                put(tone(note(m), 0.35, release=0.14, harmonics=(1, .25, .08)), te, 0.085, pan=-0.2 if e else 0.2)
    t0 += 4 * beat
    bars += 1

# Part C, 18-20 s: a bright C major chord that rings out.
for m, p in ((48, 0), (60, -.3), (64, .3), (67, -.2), (72, .2), (76, 0)):
    put(tone(note(m), 2.0, release=0.9, attack=0.01), 18.0, 0.07, pan=p)

# ---------------- sound effects ----------------
def pop(at, gain=0.35, pitch=1.0):
    put(sweep(500 * pitch, 1100 * pitch, 0.07, release=0.03), at, gain)


def whoosh(at, gain=0.18):
    n = noise(0.45, 0.5)
    n *= np.sin(np.pi * np.arange(len(n)) / len(n))
    put(lowpass(lowpass(n, 0.06), 0.06), at - 0.15, gain * 2.2)  # filtered twice: soft, not hissy


def ding(at, m=84, gain=0.28):
    put(tone(note(m), 0.9, release=0.35, harmonics=(1, 0, .3, 0, .12)), at, gain)


# 1. hook: speech bubbles and the "চেনা দৃশ্য?" label
pop(0.25)
pop(1.45, pitch=0.9)
pop(1.9, 0.3, 1.25)
# 2. problem: whoosh in, a fast ticking clock, a soft thud for each waiting row
whoosh(3.0)
for k in range(int(3.6 / 0.18)):
    put(lowpass(noise(0.02, 0.006), 0.5), 3.2 + k * 0.18, 0.5 if k % 2 == 0 else 0.35, pan=0.15)
for at in (3.7, 4.4, 5.1):
    put(tone(note(43), 0.25, release=0.08, harmonics=(1, .4)), at, 0.32)
# 3. solution: whoosh, QR card pops, scan sweep, success beep, tap, ding, status chime
whoosh(7.0)
pop(7.05, 0.3)
put(sweep(600, 1400, 0.95), 7.8, 0.09)
for k, at in enumerate((8.62, 8.74)):
    put(tone(1568 if k else 1319, 0.1, release=0.05, harmonics=(1,)), at, 0.22)
pop(8.95, 0.3, 1.2)                                    # "কোনো অ্যাপ লাগবে না"
put(lowpass(noise(0.03, 0.01), 0.7), 9.8, 0.6)         # finger taps "+"
ding(10.05, 88, 0.22)                                  # added ✓
put(sweep(300, 700, 0.18), 10.2, 0.12)                 # cart bar slides up
for k, m in enumerate((72, 76, 79)):                   # order placed: status screen
    ding(11.3 + k * 0.09, m, 0.16)
pop(12.4, 0.28, 0.8)                                   # the plate
# 4. owner: whoosh, kitchen bell, ticket slides, rising notes with the sales bars
whoosh(14.0)
put(tone(1760, 1.4, release=0.5, harmonics=(1, 0, .45, 0, .2, .1)), 14.5, 0.22, pan=0.4)  # bell
for at in (14.5, 15.0, 15.5):
    put(sweep(900, 400, 0.12), at + 0.05, 0.08, pan=0.3)
for k, m in enumerate((72, 74, 76, 79, 84)):
    put(tone(note(m), 0.25, release=0.12, harmonics=(1, .2)), 15.7 + k * 0.15, 0.14)
# 5. end card: whoosh, logo pop with a chime, button pop
whoosh(18.0, 0.2)
pop(18.1, 0.35, 0.8)
for k, m in enumerate((72, 76, 79, 84)):
    ding(18.15 + k * 0.08, m, 0.13)
pop(18.8, 0.3, 1.1)

# ---------------- master ----------------
mix = np.stack([left, right], axis=1)
fade = np.minimum(1, np.minimum(np.arange(N) / (0.03 * RATE), (N - np.arange(N)) / (0.6 * RATE)))
mix *= fade[:, None]
mix /= max(1e-9, np.abs(mix).max()) / 0.89  # peak at about -1 dB
pcm = (mix * 32767).astype(np.int16)
with wave.open(sys.argv[1] if len(sys.argv) > 1 else "bosho-sound.wav", "wb") as w:
    w.setnchannels(2)
    w.setsampwidth(2)
    w.setframerate(RATE)
    w.writeframes(pcm.tobytes())
print("wrote", len(pcm) / RATE, "seconds")
