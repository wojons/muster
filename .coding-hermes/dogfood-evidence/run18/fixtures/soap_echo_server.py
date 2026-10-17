"""Minimal SOAP 1.1 echo endpoint for dogfood probe: POST /echo (and /)."""
from flask import Flask, request, Response

app = Flask(__name__)

NS = "http://example.com/echo"

RESP = """<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <EchoResponse xmlns="{ns}"><message>{msg}</message></EchoResponse>
  </soapenv:Body>
</soapenv:Envelope>"""


def _echo():
    body = request.get_data(as_text=True)
    # crude parse: pull the first <message>...</message> content
    msg = "unknown"
    if "<message>" in body and "</message>" in body:
        msg = body.split("<message>")[1].split("</message>")[0]
    return Response(RESP.format(ns=NS, msg=msg), content_type="text/xml; charset=utf-8")


app.add_url_rule("/echo", "echo", _echo, methods=["POST"])
app.add_url_rule("/", "echo_root", _echo, methods=["POST"])

if __name__ == "__main__":
    import sys
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 9999
    app.run(port=port)
