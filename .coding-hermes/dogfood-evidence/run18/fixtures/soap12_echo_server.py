"""SOAP 1.2 echo server: POST / accepts application/soap+xml, echoes message."""
from flask import Flask, request, Response
import re

app = Flask(__name__)
NS = "http://example.com/echo"

RESP = """<?xml version="1.0" encoding="UTF-8"?>
<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope">
  <env:Body>
    <EchoResponse xmlns="{ns}"><message>{msg}</message></EchoResponse>
  </env:Body>
</env:Envelope>"""


@app.route("/echo12", methods=["POST"])
def echo12():
    body = request.get_data(as_text=True)
    m = re.search(r"<message>(.*?)</message>", body, re.S)
    msg = m.group(1) if m else "unknown"
    return Response(RESP.format(ns=NS, msg=msg), content_type='application/soap+xml; charset=utf-8; action="http://example.com/echo/Echo"')


if __name__ == "__main__":
    app.run(port=9998)
