"""Spike S2 (código desechable): sirve build/ en 127.0.0.1:8765 durante 10 minutos como máximo."""
import functools
import http.server
import pathlib
import threading

ROOT = pathlib.Path(__file__).parent / "build"
handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=str(ROOT))
server = http.server.ThreadingHTTPServer(("127.0.0.1", 8765), handler)
threading.Timer(600, server.shutdown).start()
print("sirviendo", ROOT, "en http://127.0.0.1:8765 (10 min)")
server.serve_forever()
