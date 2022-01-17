import requests
import urllib
import json
import os
import re
from dotenv import load_dotenv

# Check if on Heroku or local
# and get respected env keys
if 'DYNO' not in os.environ:
  load_dotenv()
  AUTH_KEY = os.getenv('AUTH_KEY')
  SERVER_ID = os.getenv('SERVER_ID')
else:
  AUTH_KEY = os.environ['auth_key']
  SERVER_ID = os.environ['server_id']

template = '$time | Player "$gamertag" (id=$playerID pos=<$pos>)'
logFlags = [
  "disconnected",
  ") placed ",
  "connected",
  "hit by",
  "regained consciousne",
  "is unconscious",
  "killed by",
  ")Built ",
  ") folded",
  ")Player SurvivorBase",
  ") died.",
  ") committed suicide",
  ")Dismantled",
  ") bled"
]
players = {
  'players': []
}


# Download Raw Logs off Nitrado
def getRawLogs():
  data = requests.get(
          f"https://api.nitrado.net/services/{SERVER_ID}/gameservers/file_server/download",
          headers={
            "Authorization": AUTH_KEY
          }, json={
            "file": "/games/ni5350965_2/noftp/dayzxb/config/DayZServer_X1_x64.ADM"
          }).json()

  url = data['data']['token']['url']
  urllib.request.urlretrieve(url, "logs.ADM")


# Convert Raw Logs into cleaned logs (only positional data logs)
def cleanLogs():
  with open("logs.ADM", "r") as logs:
    lines = logs.readlines()
  # Isolate Player logs (Removes Connect, Disconnect, place, hit)
  with open("clean.txt", "w") as logs:
    for line in lines:
      if not any(flag in line for flag in logFlags) and "| Player" in line.strip("\n"):
        logs.write(line)


# Generate List of player names, id's and positions
def collectPlayerData():
  with open('clean.txt', 'r') as logs:
    cleanLines = logs.readlines()
    for line in cleanLines:
      pattern = re.escape(template)
      pattern = re.sub(r'\\\$(\w+)', r'(?P<\1>.*)', pattern)
      data = re.match(pattern, line)
      if data is None: break

      query = {
        'gamertag': data.groupdict()['gamertag'],
        'playerID': data.groupdict()['playerID'],
        'time': data.groupdict()['time']+' EST',
        'pos': data.groupdict()['pos'].split(", "),
        'posHistory': []
      }

      if len(players['players'])==0: players['players'].append(query)
      else:
        for i in range(len(players['players'])):
          if players['players'][i]['gamertag']==data.groupdict()['gamertag']:
            # Updates existing player data
            for j in range(len(players['players'][i]['posHistory'])):
              query['posHistory'].append({
                'time': players['players'][i]['posHistory'][j]['time'],
                'pos': players['players'][i]['posHistory'][j]['pos']
              })

            query['posHistory'].append({
              'time': players['players'][i]['time'],
              'pos': players['players'][i]['pos']
            })

            players['players'].remove(players['players'][i])
            break

        # Logs new player data
        players['players'].append(query)


# Search Logs for Connected and Disconnected messages
def activeStatus():
  with open("logs.ADM", "r") as logs:
    lines = logs.readlines()
    for line in lines:
      status = ""
      update = False
      if "\" is connected" in line.strip("\n") and "| Player" in line.strip("\n"):
        status = "Online"
        update = True
      elif ") has been disconnected" in line.strip("\n") and "| Player" in line.strip("\n"):
        status = "Offline"
        update = True

      if update:
        beginPlayer = 19 # Player names always start here
        if status=="Online": endPlayer = line.strip("\n").find("\" is")
        if status=="Offline": endPlayer = line.strip("\n").find("\"(id=")
        playerName = line.strip("\n")[beginPlayer:endPlayer]

        playerFoundAndUpdated = False
        for i in range(len(players['players'])):
          if players['players'][i]['gamertag']==playerName:
            players['players'][i]['connectionStatus'] = status
            playerFoundAndUpdated = True
        
        if not playerFoundAndUpdated:
          # Get player ID
          beginID = line.strip("\n").find('(id=')+4
          endID = line.strip("\n").find(")")
          playerID = line.strip("\n")[beginID:endID]
          query = {
            "gamertag": playerName,
            "playerID": playerID,
            "time": None,
            "pos": [],
            "posHistory": [],
            "connectionStatus": "Online"
          }
          # Logs new player data
          players["players"].append(query)

if __name__ == '__main__':
  # getRawLogs()
  # cleanLogs()
  # collectPlayerData()
  activeStatus()

  with open("players.json", "w") as playerJSON:
    json.dump(players, playerJSON, ensure_ascii=False, indent=2)
