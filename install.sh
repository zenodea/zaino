#!/bin/sh
#
# Install zaino.
#
#   ./install.sh                            build the checkout you are standing in
#   curl -fsSL <raw-url>/install.sh | sh    clone, build, install
#
#   -d, --dir DIR     where the binary goes  (default ~/.local/bin)
#   -r, --ref REF     branch, tag or commit to clone   (default main)
#       --no-anim     skip the animation
#       --uninstall   remove the binary, keep the data
#       --purge       remove the binary and ~/.local/share/zaino
#   -h, --help        this
#
# Environment: ZAINO_INSTALL_DIR, ZAINO_REF, ZAINO_REPO, ZAINO_NO_ANIM,
# ZAINO_ANIM_H (cap the pack's height in rows), NO_COLOR, GOFLAGS.

set -eu

# A quoted --dir '~/bin' never reached the shell's tilde expansion.
untilde() {
  case $1 in
  "~") printf '%s' "$HOME" ;;
  "~/"*) printf '%s/%s' "$HOME" "${1#\~/}" ;;
  *) printf '%s' "$1" ;;
  esac
}

REPO=${ZAINO_REPO:-https://github.com/zenodea/Zaino.git}
GO_MIN=1.26.5

DIR=$(untilde "${ZAINO_INSTALL_DIR:-$HOME/.local/bin}")
REF=${ZAINO_REF:-main}
ACTION=install
ANIM=yes
TMP=
CURSOR_HIDDEN=
TTY_SAVED=
SRC_LOCAL=
ESC=
UTF=no
HEAD_H=3
FOOT_H=2
HUD_GUT=3
# Tenths: the frame is 1.6 cells wide per cell of height. The pack's drawn width
# follows the frame's height, not its width — widening the frame only buys empty
# columns — so this is the narrowest frame the pack still turns inside of.
ANIM_RATIO=16
# The pack scales with the window, but only to here. Past it the thing stops
# reading as an illustration and starts reading as wallpaper.
ANIM_MAX_H=20
ANIM_PAD=0
COLS=$(tput cols 2>/dev/null || echo 80)

say() { printf '%s\n' "$*"; }
step() { printf '  %s\n' "$*"; }
die() {
  printf 'install.sh: %s\n' "$*" >&2
  exit 1
}

have() { command -v "$1" >/dev/null 2>&1; }

# The HUD borrows the pack's palette: brass teeth, charcoal tape.
hud_setup() {
  ESC=$(printf '\033')
  case ${LC_ALL:-${LC_CTYPE:-${LANG:-}}} in
  *UTF-8* | *utf-8* | *UTF8* | *utf8*) UTF=yes ;;
  *) UTF=no ;;
  esac

  C_OFF= C_DIM= C_TXT= C_HOT= C_TEETH= C_TAPE= C_BAD=
  [ -t 1 ] || return 0
  case $(anim_color) in
  256)
    C_OFF=$ESC'[0m' C_DIM=$ESC'[38;5;244m' C_TXT=$ESC'[38;5;252m'
    C_HOT=$ESC'[38;5;214m' C_TEETH=$ESC'[38;5;208m' C_TAPE=$ESC'[38;5;240m'
    C_BAD=$ESC'[38;5;203m'
    ;;
  16)
    C_OFF=$ESC'[0m' C_DIM=$ESC'[1;30m' C_TXT=$ESC'[0;37m'
    C_HOT=$ESC'[1;33m' C_TEETH=$ESC'[0;33m' C_TAPE=$ESC'[1;30m'
    C_BAD=$ESC'[1;31m'
    ;;
  esac
}

# Whether we are standing in a checkout has to be known before the work starts:
# the header names the source, and install_work reads the same answer.
detect_src() {
  self=$(cd "$(dirname "$0")" 2>/dev/null && pwd || true)
  if [ -n "$self" ] && [ -f "$self/go.mod" ] && [ -d "$self/cmd/zaino" ]; then
    SRC_LOCAL=$self
  fi
}

# A header line that wraps is a row the cursor arithmetic does not know about,
# and every frame after it lands one row high, into the pack.
clip() {
  max=$((COLS - 4))
  if [ "${#1}" -le "$max" ]; then
    printf '%s' "$1"
    return
  fi
  printf '%s' "$1" | cut -c "1-$max"
}

# HEAD_H lines, and the animation's height is budgeted against that.
header() {
  if [ -t 1 ] && [ "${TERM:-dumb}" != dumb ]; then
    printf '\033[2J\033[3J\033[H'
  fi

  gover=$(go env GOVERSION 2>/dev/null || true)
  [ -n "$gover" ] || gover="no go on PATH"
  host="$(uname -s 2>/dev/null | tr 'A-Z' 'a-z')/$(uname -m 2>/dev/null)"
  if [ -n "$SRC_LOCAL" ]; then
    origin="checkout · $SRC_LOCAL"
  else
    origin="${REPO#https://} · $REF"
  fi

  printf '  %szaino%s %s— an agent harness in Go%s\n' \
    "$C_HOT" "$C_OFF" "$C_DIM" "$C_OFF"
  printf '  %ssource%s  %s%s%s\n' \
    "$C_DIM" "$C_OFF" "$C_TXT" "$(clip "$origin → $DIR/zaino")" "$C_OFF"
  printf '  %ssystem%s  %s%s%s\n' \
    "$C_DIM" "$C_OFF" "$C_TXT" "$(clip "$host · $gover")" "$C_OFF"
}

# Every width of the zipper is spelled out up front: the draw loop has no time
# to build strings a character at a time, and no arrays to keep them in.
zip_build() {
  # anim_ok sizes the zipper first and hangs the pack off it; without it (no
  # animation, no layout pass) fall back to the terminal.
  [ -n "${BARW:-}" ] || BARW=$((COLS - 2 * HUD_GUT))
  [ "$BARW" -gt 62 ] && BARW=62
  [ "$BARW" -lt 16 ] && BARW=16

  HUD_IND=
  i=0
  while [ "$i" -lt "$HUD_GUT" ]; do
    HUD_IND=$HUD_IND' '
    i=$((i + 1))
  done

  if [ "$UTF" = yes ]; then
    ZT='╫' ZS='█' ZS2='▓'
  else
    ZT='#' ZS='>' ZS2=')'
  fi

  acc= i=0
  while [ "$i" -le "$BARW" ]; do
    eval "ZC_$i=\$acc"
    acc=$acc$ZT
    i=$((i + 1))
  done
}

# $1 percent, $2 label, $3 elapsed, $4 label colour, $5 slider glyph.
draw_hud() {
  n=$(($1 * BARW / 100))
  [ "$n" -gt "$BARW" ] && n=$BARW
  eval "zc=\$ZC_$n"
  [ "$n" -lt "$BARW" ] && slide=$5 || slide=

  printf '\033[2K%s%s%s%s%s%s\n' \
    "$HUD_IND" "$C_TEETH" "$zc" "$C_HOT" "$slide" "$C_OFF"
  printf '\033[2K%s%s%3s%%%s  %s%s%s  %s%s%s\n' \
    "$HUD_IND" "$C_HOT" "$1" "$C_OFF" "$4" "$2" "$C_OFF" "$C_DIM" "$3" "$C_OFF"
}

# Every step must tolerate failure: under set -e, killing an already-dead process
# would abandon the rest of the cleanup and take the exit status with it.
cleanup() {
  if [ -n "$TTY_SAVED" ]; then stty "$TTY_SAVED" </dev/tty 2>/dev/null || true; fi
  if [ -n "$CURSOR_HIDDEN" ]; then printf '\033[?25h'; fi
  if [ -n "${KEY_PID:-}" ]; then kill "$KEY_PID" 2>/dev/null || true; fi
  if [ -n "$TMP" ]; then rm -rf "$TMP"; fi
  return 0
}
trap cleanup EXIT INT TERM

usage() {
  awk 'NR > 1 { if ($0 !~ /^#/) exit; sub(/^# ?/, ""); print }' "$0"
}

while [ $# -gt 0 ]; do
  case $1 in
  -d | --dir)
    [ $# -ge 2 ] || die "--dir needs a directory"
    DIR=$(untilde "$2")
    shift 2
    ;;
  -r | --ref)
    [ $# -ge 2 ] || die "--ref needs a ref"
    REF=$2
    shift 2
    ;;
  --no-anim)
    ANIM=no
    shift
    ;;
  --uninstall)
    ACTION=uninstall
    shift
    ;;
  --purge)
    ACTION=purge
    shift
    ;;
  -h | --help)
    usage
    exit 0
    ;;
  *) die "unknown option: $1 (try --help)" ;;
  esac
done

# Missing from a non-login shell's PATH is not the same as not installed.
find_go() {
  have go && return 0
  for candidate in /usr/local/go/bin /opt/homebrew/bin /usr/lib/go/bin; do
    if [ -x "$candidate/go" ]; then
      PATH=$candidate:$PATH
      export PATH
      return 0
    fi
  done
  return 1
}

older() {
  [ "$1" = "$2" ] && return 1
  [ "$(printf '%s\n%s\n' "$1" "$2" | sort -t. -k1,1n -k2,2n -k3,3n | head -1)" = "$1" ]
}

# Rays are rotated into object space rather than the geometry into world space,
# so every box stays a plain slab test; face normals are bent toward the edges of
# their face, which shades a box like something stuffed rather than like a crate.
backpack_program() {
  cat <<'AWK'
function abs(x) { return x < 0 ? -x : x }

function addbox(m, x0, x1, y0, y1, z0, z1, hinge, bev, sy0, sy1) {
	N++
	BX0[N] = x0; BX1[N] = x1
	BY0[N] = y0; BY1[N] = y1
	BZ0[N] = z0; BZ1[N] = z1
	BM[N] = m; BH[N] = hinge; BV[N] = bev
	# Boxes stacked to fake a taper share a shading range, or the bevel bands at
	# the seam between them.
	BSY0[N] = (sy0 == "" ? y0 : sy0)
	BSY1[N] = (sy1 == "" ? y1 : sy1)
}

function build() {
	N = 0
	addbox(1, -0.72, 0.72, -1.00, 0.20, -0.40, 0.40, 0, 0.68, -1.00, 0.84)
	addbox(1, -0.66, 0.66, 0.20, 0.84, -0.37, 0.36, 0, 0.68, -1.00, 0.84)
	addbox(3, -0.46, 0.46, -0.78, -0.12, 0.40, 0.55, 0, 0.85)   # front pocket
	addbox(5, -0.04, 0.04, -0.20, -0.12, 0.55, 0.59, 0, 0.20)   # zip pull
	addbox(3, -0.79, -0.72, -0.68, -0.16, -0.24, 0.24, 0, 0.80) # side pocket
	addbox(3, 0.72, 0.79, -0.68, -0.16, -0.24, 0.24, 0, 0.80)   # side pocket
	addbox(4, -0.44, -0.16, -0.90, 0.74, -0.55, -0.42, 0, 0.70) # strap
	addbox(4, 0.16, 0.44, -0.90, 0.74, -0.55, -0.42, 0, 0.70)   # strap
	# The handle stands behind the lid, not through it, or they fight for space.
	addbox(4, -0.17, -0.10, 0.78, 1.00, -0.53, -0.43, 0, 0.45)
	addbox(4, 0.10, 0.17, 0.78, 1.00, -0.53, -0.43, 0, 0.45)
	addbox(4, -0.17, 0.17, 1.00, 1.06, -0.53, -0.43, 0, 0.45)
	# Hinge-local coordinates: a plate over the top, a lip hanging down the front.
	addbox(2, -0.68, 0.68, -0.06, 0.04, 0.02, 0.82, 1, 0.30)
	addbox(2, -0.68, 0.68, -0.30, 0.04, 0.80, 0.90, 1, 0.35)
	addbox(5, -0.08, 0.08, -0.26, -0.16, 0.90, 0.94, 1, 0.30)
}

# A sphere around each box in world space. Most pixels are background, and a
# pixel that does reach the pack still misses eleven of the fourteen boxes; the
# sphere answers that for a tenth of what the slab test costs.
function spheres(   b, cx, cy, cz, dx, dy, dz, ty) {
	for (b = 1; b <= N; b++) {
		cx = (BX0[b] + BX1[b]) / 2
		cy = (BY0[b] + BY1[b]) / 2
		cz = (BZ0[b] + BZ1[b]) / 2
		dx = BX1[b] - BX0[b]
		dy = BY1[b] - BY0[b]
		dz = BZ1[b] - BZ0[b]
		if (BH[b]) {
			ty = cy * CF - cz * SF; cz = cy * SF + cz * CF; cy = ty
			cy += HY; cz += HZ
		}
		CX[b] = cx; CY[b] = cy; CZ[b] = cz
		CR2[b] = (dx * dx + dy * dy + dz * dz) / 4
	}
}

# Returns the hit distance, 0 for a miss, and leaves the normal in HN*.
function hit(b, ox, oy, oz, dx, dy, dz,
	     px, py, pz, qx, qy, qz, t0, t1, ta, tb, ax, sg, tmp,
	     ix, iy, iz, u, v, k, nx, ny, nz, len)
{
	px = ox; py = oy; pz = oz
	qx = dx; qy = dy; qz = dz
	if (BH[b]) {
		py -= HY; pz -= HZ
		tmp = py * CF + pz * SF; pz = -py * SF + pz * CF; py = tmp
		tmp = qy * CF + qz * SF; qz = -qy * SF + qz * CF; qy = tmp
	}

	t0 = -1e9; t1 = 1e9; ax = 0; sg = 0

	if (abs(qx) < 1e-9) { if (px < BX0[b] || px > BX1[b]) return 0 }
	else {
		ta = (BX0[b] - px) / qx; tb = (BX1[b] - px) / qx
		if (ta > tb) { tmp = ta; ta = tb; tb = tmp; tmp = 1 } else tmp = -1
		if (ta > t0) { t0 = ta; ax = 1; sg = tmp }
		if (tb < t1) t1 = tb
	}
	if (abs(qy) < 1e-9) { if (py < BY0[b] || py > BY1[b]) return 0 }
	else {
		ta = (BY0[b] - py) / qy; tb = (BY1[b] - py) / qy
		if (ta > tb) { tmp = ta; ta = tb; tb = tmp; tmp = 1 } else tmp = -1
		if (ta > t0) { t0 = ta; ax = 2; sg = tmp }
		if (tb < t1) t1 = tb
	}
	if (abs(qz) < 1e-9) { if (pz < BZ0[b] || pz > BZ1[b]) return 0 }
	else {
		ta = (BZ0[b] - pz) / qz; tb = (BZ1[b] - pz) / qz
		if (ta > tb) { tmp = ta; ta = tb; tb = tmp; tmp = 1 } else tmp = -1
		if (ta > t0) { t0 = ta; ax = 3; sg = tmp }
		if (tb < t1) t1 = tb
	}
	if (t1 < t0 || t0 < 0) return 0

	ix = px + t0 * qx; iy = py + t0 * qy; iz = pz + t0 * qz
	k = BV[b]
	nx = 0; ny = 0; nz = 0

	if (ax == 1) {
		nx = sg
		u = (2 * iy - BSY0[b] - BSY1[b]) / (BSY1[b] - BSY0[b])
		v = (2 * iz - BZ0[b] - BZ1[b]) / (BZ1[b] - BZ0[b])
		ny = u * k; nz = v * k
	} else if (ax == 2) {
		ny = sg
		u = (2 * ix - BX0[b] - BX1[b]) / (BX1[b] - BX0[b])
		v = (2 * iz - BZ0[b] - BZ1[b]) / (BZ1[b] - BZ0[b])
		nx = u * k; nz = v * k
	} else {
		nz = sg
		u = (2 * ix - BX0[b] - BX1[b]) / (BX1[b] - BX0[b])
		v = (2 * iy - BSY0[b] - BSY1[b]) / (BSY1[b] - BSY0[b])
		nx = u * k; ny = v * k
	}

	if (BH[b]) {
		tmp = ny * CF - nz * SF; nz = ny * SF + nz * CF; ny = tmp
	}
	len = sqrt(nx * nx + ny * ny + nz * nz)
	HNX = nx / len; HNY = ny / len; HNZ = nz / len
	return t0
}

BEGIN {
	if (W == 0) W = 64
	if (H == 0) H = 24
	if (COLOR == "") COLOR = 256

	PI = 3.14159265358979
	ESC = sprintf("%c", 27)
	# Padding each row here rather than in the shell: the draw loop reads the
	# frame as one blob and has no cheap way to indent it line by line.
	if (PAD == "") PAD = 0
	IND = ""
	for (pi = 0; pi < PAD; pi++) IND = IND " "
	RAMP = "..::--~~==++**##%%@@"
	NRAMP = length(RAMP)
	PERIOD = 32                   # frames per revolution
	DONEF = TMPD "/code"          # the worker writes it when the work is over

	build()

	split("52 88 124 130 166 202 208 214", P1, " ")     # sack: rust
	split("94 130 166 172 208 214 220 222", P2, " ")     # lid: a lighter canvas
	split("58 94 130 136 172 178 214 221", P3, " ")     # pockets
	split("232 234 236 239 242 245 249 253", P4, " ")   # straps: charcoal
	split("94 136 178 214 220 226 227 230", P5, " ")    # buckles: brass
	split("0;31 0;31 0;31 1;31 1;31 1;31 1;33 1;33", Q1, " ")
	split("0;31 0;31 1;31 1;31 1;31 1;33 1;33 1;33", Q2, " ")
	split("0;33 0;33 0;33 1;33 1;33 1;33 1;37 1;37", Q3, " ")
	split("0;30 0;30 1;30 1;30 1;30 0;37 0;37 1;37", Q4, " ")
	split("0;33 0;33 1;33 1;33 1;33 1;37 1;37 1;37", Q5, " ")
	SPARK = (COLOR == 256 ? ESC "[38;5;229m" : (COLOR == "none" ? "" : ESC "[1;33m"))

	# Looking down on the pack makes its top a lit surface, not a silhouette edge.
	PITCH = 0.30
	SP = sin(PITCH); CP = cos(PITCH)
	EYED = 5.4
	EY = EYED * SP; EZ = EYED * CP
	FOCAL = 4.3
	# The pack reaches 1.10 below its centre, and a row of gutter keeps it off
	# the header and the zipper.
	HALFH = 1.10 * H / (H - 2)
	HALFW = HALFH * (W / H) * 0.5   # terminal cells are about twice as tall as wide

	LX = -0.42; LY = 0.72; LZ = 0.66
	l = sqrt(LX * LX + LY * LY + LZ * LZ)
	LX /= l; LY /= l; LZ /= l

	HY = 0.84; HZ = -0.37           # the hinge runs along the back of the top edge

	NSP = split("0.10 0.90 0.16 0.86 0.50", SPX, " ")
	split("0.30 0.24 0.72 0.68 0.06", SPY, " ")

	# The lid stays shut, so the hinge transform is the identity.
	CF = 1; SF = 0
	spheres()
	WHOLE = 2.45                  # one sphere around the whole pack

	done = 0; spin = -1
	for (f = 0; ; f++) {
		# The pack faces front and floats while the work runs; it turns to show
		# itself off once the work is done, and stops facing front again.
		if (!done) {
			if ((getline junk < DONEF) >= 0) { done = 1; spin = 0 }
			close(DONEF)
		} else spin++

		theta = (done ? 2 * PI * (spin % PERIOD) / PERIOD : 0)
		front = (done && spin > 0 && spin % PERIOD == 0)
		BOB = -0.03 + (done ? 0.05 : 0.10) * sin(f * 0.38)
		CT = cos(theta); ST = sin(theta)

		# A header the player consumes but does not draw: the spin is left facing
		# front, so it has to know which frame that is.
		print "F " f " " (front ? "front" : "-")
		sparkles(f)
		render()
		fflush()
	}
}

function sparkles(f,   s, ph, c) {
	split("", SPK)
	for (s = 1; s <= NSP; s++) {
		ph = (f + 3 * s) % 14
		c = substr(".+*+.         ", ph + 1, 1)
		if (c != " ") SPK[int(SPX[s] * W) "," int(SPY[s] * H)] = c
	}
}

function render(   i, j, sx, sy, dx, dy, dz, len, ox, oy, oz, rx, ry, rz,
		   b, t, best, bm, nx, ny, nz, ndl, hx, hy, hz, spec, inten,
		   row, ch, idx, col, last, wx, wy, wz, key, ty, tz,
		   ex, ey, ez, tca, ODIST)
{
	ox = -EZ * ST; oy = EY; oz = EZ * CT
	ODIST = ox * ox + oy * oy + oz * oz
	for (j = 0; j < H; j++) {
		row = ""; last = ""
		for (i = 0; i < W; i++) {
			sx = (2 * (i + 0.5) / W - 1) * HALFW
			sy = (1 - 2 * (j + 0.5) / H) * HALFH - BOB
			dx = sx; dy = sy; dz = -FOCAL
			len = sqrt(dx * dx + dy * dy + dz * dz)
			dx /= len; dy /= len; dz /= len

			ty = dy * CP + dz * SP
			tz = -dy * SP + dz * CP

			rx = dx * CT - tz * ST
			rz = dx * ST + tz * CT
			ry = ty

			best = 0; bm = 0
			tca = -(ox * rx + oy * ry + oz * rz)
			if (tca > 0 && ODIST - tca * tca < WHOLE) {
				for (b = 1; b <= N; b++) {
					ex = CX[b] - ox
					ey = CY[b] - oy
					ez = CZ[b] - oz
					tca = ex * rx + ey * ry + ez * rz
					if (tca <= 0) continue
					if (ex * ex + ey * ey + ez * ez - tca * tca > CR2[b]) continue
					t = hit(b, ox, oy, oz, rx, ry, rz)
					if (t > 0 && (best == 0 || t < best)) {
						best = t; bm = BM[b]
						nx = HNX; ny = HNY; nz = HNZ
					}
				}
			}

			if (best == 0) {
				key = i "," j
				if (key in SPK) {
					if (SPARK != last) { row = row SPARK; last = SPARK }
					row = row SPK[key]
				} else {
					row = row " "
				}
				continue
			}

			wx = nx * CT + nz * ST
			wz = -nx * ST + nz * CT
			wy = ny

			ndl = wx * LX + wy * LY + wz * LZ
			if (ndl < 0) ndl = 0
			hx = LX; hy = LY + SP; hz = LZ + CP
			len = sqrt(hx * hx + hy * hy + hz * hz)
			spec = (wx * hx + wy * hy + wz * hz) / len
			if (spec < 0) spec = 0
			spec = spec * spec; spec = spec * spec
			spec = spec * spec; spec = spec * spec * 0.60

			inten = 0.16 + 0.68 * ndl + spec
			if (inten > 1) inten = 1

			idx = int(inten * (NRAMP - 1)) + 1
			ch = substr(RAMP, idx, 1)
			col = shade(bm, inten)
			if (col != last) { row = row col; last = col }
			row = row ch
		}
		if (last != "" && COLOR != "none") row = row ESC "[0m"
		print IND row
	}
}

function shade(m, inten,   k) {
	if (COLOR == "none") return ""
	k = int(inten * 7.999) + 1
	if (k > 8) k = 8
	if (COLOR == 256) {
		if (m == 1) return ESC "[38;5;" P1[k] "m"
		if (m == 2) return ESC "[38;5;" P2[k] "m"
		if (m == 3) return ESC "[38;5;" P3[k] "m"
		if (m == 4) return ESC "[38;5;" P4[k] "m"
		return ESC "[38;5;" P5[k] "m"
	}
	if (m == 1) return ESC "[" Q1[k] "m"
	if (m == 2) return ESC "[" Q2[k] "m"
	if (m == 3) return ESC "[" Q3[k] "m"
	if (m == 4) return ESC "[" Q4[k] "m"
	return ESC "[" Q5[k] "m"
}
AWK
}

anim_ok() {
  [ -z "${ZAINO_NO_ANIM:-}" ] || return 1
  [ "$ANIM" = yes ] || return 1
  [ -t 1 ] || return 1
  have awk || return 1
  case ${TERM:-dumb} in dumb | "") return 1 ;; esac
  sleep 0.04 2>/dev/null || return 1 # no fractional sleep, no animation

  lines=$(tput lines 2>/dev/null || echo 24)
  cols=$(tput cols 2>/dev/null || echo 80)

  # The zipper is the widest thing drawn, and the pack sits centred over it, so
  # the layout is measured from the zipper outwards.
  BARW=$((cols - 2 * HUD_GUT))
  [ "$BARW" -gt 62 ] && BARW=62

  ANIM_H=$((lines - HEAD_H - FOOT_H - 1))
  [ "$ANIM_H" -gt "$ANIM_MAX_H" ] && ANIM_H=$ANIM_MAX_H
  case ${ZAINO_ANIM_H:-} in
  '' | *[!0-9]*) ;;
  *) [ "$ZAINO_ANIM_H" -gt 0 ] && [ "$ANIM_H" -gt "$ZAINO_ANIM_H" ] &&
    ANIM_H=$ZAINO_ANIM_H ;;
  esac
  ANIM_W=$((ANIM_H * ANIM_RATIO / 10))

  # The pack is drawn to fill its frame, so trimming the width alone does not
  # crop it — it fattens it. A frame that will not fit has to cost height too,
  # or a narrow terminal gets a pack swollen to the size of the window.
  if [ "$ANIM_W" -gt "$BARW" ]; then
    ANIM_W=$BARW
    ANIM_H=$((ANIM_W * 10 / ANIM_RATIO))
  fi

  # Centred on the zipper rather than on the terminal: the two read as one
  # object, and the HUD keeps the left margin the header set.
  ANIM_PAD=$((HUD_GUT + (BARW - ANIM_W) / 2))
  [ "$ANIM_PAD" -lt 0 ] && ANIM_PAD=0

  # 56 columns is what the widest HUD line needs; narrower, and it wraps and
  # takes the cursor arithmetic with it.
  [ "$ANIM_H" -ge 10 ] && [ "$ANIM_W" -ge 22 ] && [ "$cols" -ge 56 ]
}

anim_color() {
  [ -z "${NO_COLOR:-}" ] || {
    echo none
    return
  }
  case ${COLORTERM:-}${TERM:-} in
  *truecolor* | *24bit* | *256color* | *kitty* | *alacritty* | *wezterm*) echo 256 ;;
  *) echo 16 ;;
  esac
}

# Turns the pack while process $1 lives, then until return is pressed.
spin_while() {
  worker=$1
  prog=$(backpack_program)
  color=$(anim_color)
  zip_build

  # Not stdin: piped into sh, stdin is the script itself and is already at EOF.
  KEY_PID=
  if [ -r /dev/tty ]; then
    # The terminal would echo the ⏎ that ends the wait, and that newline scrolls
    # the screen out from under the cursor arithmetic: every frame after it is
    # drawn a row low, and the zipper it left behind stays on screen.
    TTY_SAVED=$(stty -g </dev/tty 2>/dev/null || true)
    [ -n "$TTY_SAVED" ] && { stty -echo </dev/tty 2>/dev/null || TTY_SAVED=; }
    (
      read -r _ </dev/tty 2>/dev/null || true
      : >"$TMP/keyed"
    ) &
    KEY_PID=$!
  fi

  CURSOR_HIDDEN=yes
  printf '\033[?25l'
  begun=$(date +%s 2>/dev/null || echo 0)
  # The program is all BEGIN; /dev/null stops an awk from waiting on a terminal.
  awk -v W="$ANIM_W" -v H="$ANIM_H" -v COLOR="$color" -v PAD="$ANIM_PAD" \
    -v TMPD="$TMP" \
    "$prog" </dev/null 2>/dev/null |
    {
      first=yes
      fc=0
      cur=0
      tgt=0
      label="starting"
      lcol=$C_TXT
      lap="0:00"
      ended=no
      ok=no
      endf=0
      while :; do
        IFS= read -r head || break
        buf=
        k=0
        while [ "$k" -lt "$ANIM_H" ]; do
          IFS= read -r line || break 2
          buf=$buf$line'
'
          k=$((k + 1))
        done

        if [ "$first" = yes ]; then
          : >"$TMP/drew"
          first=no
        else
          printf '\033[%dA' "$((ANIM_H + FOOT_H))"
        fi
        printf '%s' "$buf"

        # Read it even after the worker is gone: a failure leaves the zipper
        # where the work stopped, which is the last thing it wrote.
        if [ -s "$TMP/status" ]; then
          IFS= read -r note <"$TMP/status" || note=
          # A half-written status file reads as nothing; keep the last one.
          case $note in *"|"*) tgt=${note%%|*} label=${note#*|} ;; esac
        fi

        if ! kill -0 "$worker" 2>/dev/null; then
          if [ "$ended" = no ]; then
            ended=yes
            [ "$(exitcode)" = 0 ] && ok=yes || lcol=$C_BAD
          fi
          # Only a run that worked gets to shut the zipper.
          [ "$ok" = yes ] && tgt=100
          label=$(outcome)
        fi

        if [ "$cur" -lt "$tgt" ]; then
          cur=$((cur + 1 + (tgt - cur) / 12))
          [ "$cur" -gt "$tgt" ] && cur=$tgt
        elif [ "$ended" = no ] && [ $((fc % 24)) -eq 0 ]; then
          # A long compile has to keep the zipper moving, or it reads as jammed.
          cap=$((tgt + 30))
          [ "$cap" -gt 92 ] && cap=92
          [ "$cur" -lt "$cap" ] && cur=$((cur + 1))
        fi

        if [ $((fc % 12)) -eq 0 ]; then
          secs=$(($(date +%s 2>/dev/null || echo "$begun") - begun))
          [ "$secs" -lt 0 ] && secs=0
          lap=$(printf '%d:%02d' $((secs / 60)) $((secs % 60)))
        fi

        [ $((fc / 4 % 2)) -eq 0 ] && pull=$ZS || pull=$ZS2
        draw_hud "$cur" "$label" "$lap" "$lcol" "$pull"

        # Nobody to press anything: settle shut, on the next front view.
        if [ "$ended" = yes ] && { [ "$ok" = no ] || [ "$cur" -ge 100 ]; }; then
          endf=$((endf + 1))
          if [ -z "$KEY_PID" ] || [ -f "$TMP/keyed" ]; then
            case $head in *front*) break ;; esac
            # A pack that never comes back around must not hold the shell.
            [ "$endf" -gt 120 ] && break
          fi
        fi
        fc=$((fc + 1))
        sleep 0.045
      done
    }
  printf '\033[?25h'
  CURSOR_HIDDEN=
  if [ -n "$TTY_SAVED" ]; then
    stty "$TTY_SAVED" </dev/tty 2>/dev/null || true
    TTY_SAVED=
  fi
}

# $1 is how far along the zipper should be, $2 what to call it.
status() { printf '%s|%s\n' "$1" "$2" >"$TMP/status"; }

exitcode() {
  code=1
  if [ -f "$TMP/code" ]; then
    IFS= read -r code <"$TMP/code" || code=1
  fi
  printf '%s' "$code"
}

outcome() {
  code=$(exitcode)
  [ "$code" = 0 ] && word=installed || word=failed
  if [ -n "${KEY_PID:-}" ] && [ ! -f "$TMP/keyed" ]; then
    printf '%s — press ⏎ to finish' "$word"
  else
    printf '%s' "$word"
  fi
}

remove() {
  target=$DIR/zaino
  if [ -e "$target" ]; then
    rm -f "$target"
    step "removed $target"
  else
    step "no binary at $target"
  fi

  other=$(command -v zaino 2>/dev/null || true)
  if [ -n "$other" ]; then
    step "note: another zaino is still on PATH at $other"
  fi

  if [ "$ACTION" = purge ]; then
    data=${XDG_DATA_HOME:-$HOME/.local/share}/zaino
    if [ -d "$data" ]; then
      rm -rf "$data"
      step "removed $data"
    else
      step "no data at $data"
    fi
  fi
}

# Runs in the background while the pack turns: status() narrates, step() records.
install_work() {
  status 6 "looking for go"
  find_go || die "go is not installed — see https://go.dev/dl (Go $GO_MIN or newer)"

  go_version=$(go env GOVERSION 2>/dev/null | sed 's/^go//; s/[a-z-].*//')
  step "go ${go_version:-?} at $(command -v go) — needs $GO_MIN or newer"
  if [ -n "$go_version" ] && older "$go_version" "$GO_MIN"; then
    # Modern toolchains fetch what a module asks for; older ones just fail.
    step "go $go_version is older than $GO_MIN; letting the toolchain sort it out"
  fi

  status 16 "reading the source"
  if [ -n "$SRC_LOCAL" ]; then
    src=$SRC_LOCAL
    step "building the checkout at $src"
    if [ "$REF" != "${ZAINO_REF:-main}" ]; then
      step "note: --ref $REF is ignored here; run this outside a checkout to clone"
    fi
  else
    have git || die "git is not installed, and this is not a zaino checkout"
    status 16 "cloning"
    step "cloning $REPO ($REF)"
    git clone --quiet --depth 1 --branch "$REF" "$REPO" "$TMP/src" 2>/dev/null ||
      {
        # --branch does not take a commit sha, so fall back to a full clone.
        git clone --quiet "$REPO" "$TMP/src" &&
          git -C "$TMP/src" checkout --quiet "$REF"
      } || die "could not clone $REPO at $REF"
    src=$TMP/src
    [ -f "$src/go.mod" ] ||
      die "$REPO at $REF has no go.mod — is the source pushed to that branch?"
  fi

  head_ref=$(git -C "$src" rev-parse --short HEAD 2>/dev/null || true)
  [ -n "$head_ref" ] && step "at commit $head_ref"

  mkdir -p "$DIR" || die "cannot create $DIR"
  [ -w "$DIR" ] || die "$DIR is not writable — pass --dir ~/.local/bin"

  status 46 "compiling"
  step "compiling ./cmd/zaino"
  if [ -n "${GOOS:-}${GOARCH:-}" ]; then
    step "note: ignoring GOOS/GOARCH — this installs a binary to run here"
  fi
  # Built beside the target so the rename that publishes it is atomic, and
  # without GOOS/GOARCH, which would install cleanly and then not run.
  out=$DIR/.zaino.$$
  built=$(date +%s 2>/dev/null || echo 0)
  (cd "$src" && unset GOOS GOARCH && go build -trimpath -o "$out" ./cmd/zaino) ||
    {
      rm -f "$out"
      die "build failed"
    }
  step "compiled in $(($(date +%s 2>/dev/null || echo "$built") - built))s"

  status 90 "checking the binary"

  # Flags are parsed before anything else runs, so this only proves it executes
  # here; 126 and 127 are the shell saying it could not.
  rc=0
  "$out" -h >/dev/null 2>&1 || rc=$?
  if [ "$rc" -eq 126 ] || [ "$rc" -eq 127 ]; then
    rm -f "$out"
    die "the binary that came out of the build will not run here"
  fi

  status 96 "installing"
  mv -f "$out" "$DIR/zaino" || {
    rm -f "$out"
    die "could not install into $DIR"
  }
  chmod 755 "$DIR/zaino"
  size=$(du -h "$DIR/zaino" 2>/dev/null | awk '{ print $1 }')

  status 100 "installed"
  step "installed $DIR/zaino${size:+ ($size)}"
}

finish() {
  case ":${PATH}:" in
  *":$DIR:"*) ;;
  *)
    say ""
    say "$DIR is not on your PATH. Add it:"
    case ${SHELL##*/} in
    fish) say "    fish_add_path $DIR" ;;
    zsh) say "    echo 'export PATH=\"$DIR:\$PATH\"' >> ~/.zshrc" ;;
    *) say "    echo 'export PATH=\"$DIR:\$PATH\"' >> ~/.profile" ;;
    esac
    ;;
  esac

  say ""
  say "Set a key, then run zaino:"
  say "    export ANTHROPIC_API_KEY=sk-ant-...   # console.anthropic.com"
  say "    export GEMINI_API_KEY=...             # aistudio.google.com/apikey"
  say "    zaino"
}

hud_setup
detect_src
find_go || true

if [ "$ACTION" != install ]; then
  say "Uninstalling zaino"
  remove
  say "Done."
  exit 0
fi

TMP=$(mktemp -d "${TMPDIR:-/tmp}/zaino-install.XXXXXX")
header

if anim_ok; then
  : >"$TMP/status"
  # The worker drops the parent's trap, or it deletes the temp directory its own
  # output is in, and records its status, which the pack reports before the wait.
  {
    st=0
    (
      trap - EXIT INT TERM
      install_work
    ) >"$TMP/log" 2>&1 || st=$?
    printf '%s\n' "$st" >"$TMP/code"
  } &
  worker=$!

  spin_while "$worker"
  # A broken awk draws nothing, and the wait below would be a silent one.
  if [ ! -f "$TMP/drew" ]; then
    say "Installing zaino"
  fi
  wait "$worker" 2>/dev/null || true

  code=1
  if [ -f "$TMP/code" ]; then
    IFS= read -r code <"$TMP/code" || code=1
  fi

  say ""
  if [ "$code" -eq 0 ]; then
    cat "$TMP/log"
  else
    cat "$TMP/log" >&2
    exit "$code"
  fi
  finish
else
  say "Installing zaino"
  install_work
  finish
fi
