"""Minimal ISO9660 + Joliet writer (macOS mounts it as a .dmg). usage: mkiso.py SRC OUT VOLNAME"""
import os, sys, struct, time

S = 2048
src, out, vol = sys.argv[1], sys.argv[2], sys.argv[3]

def both16(n): return struct.pack('<H', n) + struct.pack('>H', n)
def both32(n): return struct.pack('<I', n) + struct.pack('>I', n)
t = time.gmtime()
DT7 = bytes([t.tm_year - 1900, t.tm_mon, t.tm_mday, t.tm_hour, t.tm_min, t.tm_sec, 0])
DT17 = time.strftime('%Y%m%d%H%M%S00', t).encode() + b'\x00'

class Node:
    def __init__(s, path, name, parent):
        s.path, s.name, s.parent = path, name, parent
        s.isdir = os.path.isdir(path)
        s.size = 0 if s.isdir else os.path.getsize(path)
        s.kids = sorted((Node(os.path.join(path, n), n, s) for n in os.listdir(path)), key=lambda k: k.name) if s.isdir else []

root = Node(src, '', None)
dirs, files = [], []
def walk(n):  # BFS order for path tables
    q = [n]
    while q:
        d = q.pop(0); dirs.append(d)
        for k in d.kids:
            (q.append if k.isdir else files.append)(k)
walk(root)
for i, d in enumerate(dirs): d.num = i + 1
cnt = [0]
def pname(n):  # primary (8.3) names
    cnt[0] += 1
    return (b'D%05d' % cnt[0]) if n.isdir else (b'F%05d.;1' % cnt[0])
for n in dirs[1:] + files: n.pid = pname(n)
for n in dirs[1:] + files: n.jid = n.name.encode('utf-16-be')
root.pid = root.jid = b'\x00'

def rec(ident, lba, size, isdir):
    l = 33 + len(ident) + (1 - len(ident) % 2)
    return bytes([l, 0]) + both32(lba) + both32(size) + DT7 + bytes([2 if isdir else 0, 0, 0]) + both16(1) + bytes([len(ident)]) + ident + (b'\x00' if len(ident) % 2 == 0 else b'')

def dir_size(d, key):
    used, secs = 0, 1
    for r in [34, 34] + [33 + len(getattr(k, key)) + (1 - len(getattr(k, key)) % 2) for k in d.kids]:
        if used + r > S: secs += 1; used = 0
        used += r
    return secs * S

def ptable(key, big):
    b = b''
    for d in dirs:
        ident = getattr(d, key)
        b += bytes([len(ident), 0]) + struct.pack('>I' if big else '<I', d.lba[key]) + struct.pack('>H' if big else '<H', d.parent.num if d.parent else 1) + ident + (b'\x00' if len(ident) % 2 else b'')
    return b

# layout
lba = 19
pt = {}
for d in dirs: d.lba = {'pid': 0, 'jid': 0}
ptsz = {k: len(ptable(k, False)) for k in ('pid', 'jid')}
for k in ('pid', 'jid'):
    pt[k] = (lba, lba + 1); lba += 2 * ((ptsz[k] + S - 1) // S) if ptsz[k] > S else 2
for k in ('pid', 'jid'):
    for d in dirs:
        d.lba[k] = lba; d.dsize = getattr(d, 'dsize', {}); d.dsize[k] = dir_size(d, k); lba += d.dsize[k] // S
for f in files:
    f.lba = lba; lba += max(1, (f.size + S - 1) // S)
total = lba

def dir_bytes(d, key):
    recs = [rec(b'\x00', d.lba[key], d.dsize[key], True),
            rec(b'\x01', (d.parent or d).lba[key], (d.parent or d).dsize[key], True)]
    recs += [rec(getattr(k, key), k.lba[key] if k.isdir else k.lba, k.dsize[key] if k.isdir else k.size, k.isdir) for k in d.kids]
    b, cur = b'', b''
    for r in recs:
        if len(cur) + len(r) > S: b += cur.ljust(S, b'\x00'); cur = b''
        cur += r
    return (b + cur.ljust(S, b'\x00')).ljust(d.dsize[key], b'\x00')

def vd(typ, key):
    j = key == 'jid'
    txt = (lambda s, n: s.encode('utf-16-be')[:n].ljust(n, b'\x00')) if j else (lambda s, n: s.upper().encode()[:n].ljust(n, b' '))
    v = bytearray(S)
    v[0] = typ; v[1:6] = b'CD001'; v[6] = 1
    v[8:40] = txt('', 32) if j else b' ' * 32
    v[40:72] = txt(vol, 32) if j else txt('OMNISUCK', 32)
    v[80:88] = both32(total)
    if j: v[88:91] = b'%/E'
    v[120:124] = both16(1); v[124:128] = both16(1); v[128:132] = both16(S)
    v[132:140] = both32(ptsz[key])
    v[140:144] = struct.pack('<I', pt[key][0]); v[148:152] = struct.pack('>I', pt[key][1])
    v[156:190] = rec(b'\x00', root.lba[key], root.dsize[key], True)
    for o, n in ((190, 128), (318, 128), (446, 128), (574, 128), (702, 37), (739, 37), (776, 37)):
        v[o:o + n] = (b'\x00' * n) if j else b' ' * n
    v[813:830] = DT17; v[830:847] = DT17; v[847:864] = b'0' * 16 + b'\x00'; v[864:881] = b'0' * 16 + b'\x00'
    v[881] = 1
    return bytes(v)

with open(out, 'wb') as o:
    o.write(b'\x00' * 16 * S)
    o.write(vd(1, 'pid')); o.write(vd(2, 'jid'))
    o.write(bytes([255]) + b'CD001\x01' + b'\x00' * (S - 7))
    for k in ('pid', 'jid'):
        o.seek(pt[k][0] * S); o.write(ptable(k, False))
        o.seek(pt[k][1] * S); o.write(ptable(k, True))
    for k in ('pid', 'jid'):
        for d in dirs: o.seek(d.lba[k] * S); o.write(dir_bytes(d, k))
    for f in files:
        o.seek(f.lba * S); o.write(open(f.path, 'rb').read())
    o.seek(total * S - 1); o.write(b'\x00')
print('ok', total * S // 1024, 'KB')
