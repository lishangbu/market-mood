#!/usr/bin/env python3
"""Bounded independent TCP probe for the two configured TDX servers.
Uses documented pytdx packet layout; records no credentials or account data.
This diagnostic does not accept or publish malformed market data.
"""
import argparse
import datetime
import json
import socket
import struct
import zlib

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--output')
args = parser.parse_args()
handshakes = [bytes.fromhex(x) for x in [
    '0c0218930001030003000d0001',
    '0c0218940001030003000d0002',
    '0c031899000120002000db0fd5d0c9ccd6a4a8af0000008fc22540130000d500c9ccbdf0d7ea00000002',
]]
def read_exact(sock, size):
    data = b''
    while len(data) < size:
        chunk = sock.recv(size - len(data))
        if not chunk:
            raise EOFError(f'expected {size}, received {len(data)}')
        data += chunk
    return data

def request(sock, packet):
    sock.sendall(packet)
    header = read_exact(sock, 16)
    size, expanded = struct.unpack_from('<HH', header, 12)
    raw = read_exact(sock, size)
    body = raw if size == expanded else zlib.decompress(raw)
    if len(body) != expanded:
        raise ValueError('decompressed length mismatch')
    return header, body

results = []
for host in ['115.238.90.165', '180.153.18.170']:
    for code, market, count in [('600000', 1, 10), ('000001', 0, 10), ('600000', 1, 1), ('600000', 1, 800)]:
        entry = {'at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'host': host, 'code': code, 'market': market, 'requested_count': count}
        try:
            with socket.create_connection((host, 7709), timeout=5) as sock:
                sock.settimeout(5)
                for packet in handshakes:
                    request(sock, packet)
                packet = struct.pack('<HIHHHH6sHHHHIIH', 0x10c, 0x01016408, 0x1c, 0x1c, 0x052d, market, code.encode(), 9, 1, 0, count, 0, 0, 0)
                header, body = request(sock, packet)
                entry.update(request_hex=packet.hex(), header_hex=header.hex(), body_length=len(body), body_prefix_hex=body[:32].hex(), declared_count=struct.unpack_from('<H', body)[0] if len(body) >= 2 else None)
                sock.settimeout(.2)
                try:
                    extra = sock.recv(1024)
                    entry['extra_bytes'] = len(extra)
                except socket.timeout:
                    entry['extra_bytes'] = 0
        except Exception as error:
            entry['error'] = str(error)
        results.append(entry)
        print(json.dumps(entry), flush=True)
if args.output:
    with open(args.output, 'w') as output:
        json.dump(results, output, indent=2)
        output.write('\n')
