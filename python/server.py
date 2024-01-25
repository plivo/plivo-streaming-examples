## Python 3.11.4
import argparse
import asyncio
import websockets
import json
import base64
import wave
from sanic import Sanic
from sanic.response import text
import xml.etree.ElementTree as ET


app = Sanic(__name__)

sendable_event = asyncio.Event()
sendable_event.set()

def load_config(filename):
   with open(filename, 'r') as f:
       config = json.load(f)
   return config


config = load_config('config.json')

def form_json(audio_data):
   base64_audio = base64.b64encode(audio_data).decode("utf-8")
   return {
       "event": "playAudio",
       "media": {
           "contentType": config["contentType"],
           "sampleRate": config["sampleRate"],
           "payload": base64_audio
       }
   }

async def read_audio_chunks1(chunk_size_bytes):
   file_path = config["filePath"]
   with open(file_path, "rb") as audio_file:
       while True:
           # Wait till previous chunk sending is completed.
           await sendable_event.wait()
           sendable_event.clear()
           print("chunk_size_bytes :", chunk_size_bytes)
           if config["chunkingSupport"]:
               chunk = audio_file.read(chunk_size_bytes)
           else:
               chunk = audio_file.read()

           if not chunk:
               break

           yield chunk

async def send_audio_data(ws):
   try:
       content_type = config["contentType"]
       file_path = config["filePath"]
       chunk_size = config["chunkDuration"]
       if content_type == "wav":
           with wave.open(file_path, 'rb') as audio_file:
               sample_width = audio_file.getsampwidth()
               sample_rate = audio_file.getframerate()
               chunk_size_bytes = int(sample_width * sample_rate * chunk_size)
       else:
           sample_width = config["bitDepth"]
           sample_rate = config["sampleRate"]
           chunk_size_bytes = int(sample_rate * sample_width // 8 * chunk_size)

       async for chunk in read_audio_chunks1(chunk_size_bytes):
           payload = form_json(chunk)
           data = json.dumps(payload)
           print("sending data of length: ", len(data))
           await asyncio.wait_for(ws.send(data), timeout=5)
   except Exception as e:
       print("Error sending audio:", e)


def parse_and_save_to_file(json_data):
   try:
       # Parse JSON data
       data = json.loads(json_data)
       if data["event"] == "media":
           # Decode Base64 payload
           payload_base64 = data['media']['payload']
           decoded_payload = base64.b64decode(payload_base64)

           # Generate file name
           stream_id = data['streamId']
           track_type = data['media']['track']
           file_name = f"{stream_id}_{track_type}.raw"

           # Write decoded payload to a file
           with open(file_name, 'ab') as file:
               file.write(decoded_payload)
   except Exception as e:
       print("Error : recieved bad data", e)


async def receive_messages(ws):
   while True:
       try:
           message = await ws.recv()
           parse_and_save_to_file(message)
       except Exception as e:
           print("Error receiving message:", e)


async def websocket_handler(request, ws):
   try:
       tasks = [
           asyncio.create_task(receive_messages(ws)),
           asyncio.create_task(send_audio_data(ws))
       ]

       await asyncio.gather(*tasks)
   except Exception as e:
       print("[websocket_handler] exception " , e)


@app.route('/')
async def index(request):
   return text("WebSocket server is running.")

@app.route('/cb', methods=['POST'])
async def status_callback_handler(request):
   if request.form.get('Event') == "PlayedStream":
       sendable_event.set()
   # sendable_event.set()
   return text("", status=200)


#returns XML for initiating audio stream
@app.route('/xml', methods=['GET'])
async def xml(request):
   response = ET.Element("Response")
   stream = ET.SubElement(response, "Stream", bidirectional="true", statusCallbackUrl="http://"+config["publicIp"]+"/cb")
   stream.text = "ws://"+config["publicIp"]+"/ws"
   conference = ET.SubElement(response, "Conference")
   conference.text = "ConferenceTest"
   xml_string = ET.tostring(response, encoding="utf-8", method="xml")
   return text(xml_string.decode("utf-8"), content_type='application/xml')


@app.websocket('/ws')
async def websocket_route(request, ws):
   await websocket_handler(request, ws)


if __name__ == "__main__":
   parser = argparse.ArgumentParser(description="Run the Sanic server")
   parser.add_argument("--port", type=int, default=19088, help="Port number (default: 8000)")
   parser.add_argument("--chunk", type=bool, default=False, help="Chunking support (default: False)")
   args = parser.parse_args()
   chunkingSupport  = args.chunk

   app.run(host="0.0.0.0", port=args.port) 

