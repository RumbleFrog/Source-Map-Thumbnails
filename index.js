const spawn = require('child_process').spawn
    , config = require('./config.json')
    , ip = require('ip')
    , rcon = require('srcds-rcon')
    , log = require('winston')
    , path = require('path')
    , fs = require('fs')
    , _ = require('lodash')

log.addColors({ error: "red", warning: "yellow", info: "green", verbose: "white", debug: "blue" });

log.remove(log.transports.Console);

log.add(log.transports.Console, { level: config.debug_level, prettyPrint: true, colorize: true, timestamp: true });

let game_dir = config.game_directory.endsWith("/") ? config.game_directory : config.game_directory + "/"

let maps = [];

const list = {};

let index = 0;

checkFileExists('list.json')
    .then((exist) => {
        if (!exist) {
            log.error('Missing list.json file');
            process.exit(1);
        }

        fs.readFile('list.json', (err, data) => {
            if (err) {
                log.error('Unable to read list.json', err);
                process.exit(1);
            }

            let json = JSON.parse(data.toString());

            if (!Array.isArray(json)) {
                log.error('list.json Is not in array format');
                process.exit(1);
            }

            maps = json;

            log.info(`Queued ${json.length} maps`);
        })
    })

const RP = Math.random().toString(36).substring(2);

const game = spawn(config.game_binary_location, [
  `-game`, config.game,
  '-windowed',
  '-novid',
  '-usercon',
  `+map`, config.starting_map,
  `+rcon_password`, RP,
  ... [].concat(... config.launch_options.map(o => o.split(' ')))
]);

log.info(`Session RCON Password: ${RP}`)
log.info('Launching game ...');
log.info('Allowing up to 80 seconds before connection attempt');

const conn = rcon({
  address: ip.address(),
  password: RP
});

setTimeout(attemptRconConnect, 80000);

game.on('close', (code) => {
  log.info('Game has exited, terminating script');
  process.exit(0);
});

function attemptRconConnect() {
  conn.connect()
    .then(() => {
      log.info('Successfully connected to game, switching to first map ...');

      switchMap(index);
    })
    .catch((err) => {
      log.debug(err);
      log.warn('Failed to connect, retrying in 30 seconds')

      setTimeout(attemptRconConnect, 30000);
    });
}

function prepGame() {
  return new Promise((resolve, reject) => {
    log.debug('Sent status command');

    conn.command('status')
      .then((status) => {
        const m = status.match(/map\s+:\s([A-z0-9]+)/i)[1];
        const cstate = status.match(/#.* +([0-9]+) +"(.+)" +(STEAM_[0-9]:[0-9]:[0-9]+|\[U:[0-9]:[0-9]+\]) +([0-9:]+) +([0-9]+) +([0-9]+) +([a-zA-Z]+).* +([A-z0-9.:]+)/i)[7];

        if (m == maps[index] && cstate == 'active')
          setTimeout(resolve, 1000);
        else {
          log.debug('map/status failed');
          reject();
        }
      })
      .catch((err) => {})

  })
}

const throttleAttemptScreenshot = _.throttle(attemptScreenshot, 9000);

function attemptScreenshot() {
  log.debug('Attempting to screenshot');
  Promise.race([
    new Promise((madeit, tooslow) => {
      prepGame()
        .then(() => conn.command('sv_cheats 1'))
        .then(() => conn.command('cl_drawhud 0'))
        .then(() => conn.command('spec_mode'))
        .then(() => conn.command('jpeg_quality 100'))
        .then(() => getNodes())
        .then((count) => screenshot(count))
        .then((o) => {
          if (o && o.times && o.index == index) {
            madeit();
            log.info(`Screenshotted ${maps[index]} with ${o.times} spectator nodes`);
            if (index + 1 <= maps.length - 1)
              switchMap(++index);
            else {
              log.info(`Processed ${index + 1} maps`);
              process.exit(0);
            }
          }
        })
        .catch((e) => {})
    }),
    new Promise((resolve, reject) => {
      setTimeout(reject, 5000);
    })
  ]).then(() => {}).catch(() => {
      log.debug('Retrying screenshot');
      setTimeout(throttleAttemptScreenshot, 10000)
  });
}

function screenshot(times) {
  return new Promise((resolve, reject) => {
    let cm = index;
    let command = '';

    for (var i = 1; i <= times; i++)
      command += 'jpeg;wait 30;spec_next;';

    conn.command(command)
      .then(() => {
        setTimeout(() => {
          resolve({'times':times, 'index':cm});
        }, (times * 800))
      })
      .catch(() => {});
  })
}

function timeout(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

async function getNodes() {
  try {
    let going = true;
    const pos = [];
    while (going) {
      let p = await conn.command('spec_pos;spec_next');
      if (pos.includes(p)) {
        going = false;
        return pos.length;
      } else
        pos.push(p);
      await timeout(200);
    }
  } catch(e) {
    log.debug('getNodes got lost');
    // Nothing, wait for main timeout
  }
}

function switchMap(n) {

  log.debug('switchMap', n);

  if (!maps[n])
    return;

  Promise.race([
    new Promise((resolve, reject) => {
      checkMapExists(n)
        .then((exist) => {
          resolve();
          if (!exist) {
              log.warn(`${maps[n]} missing. Skipping.`);
              switchMap(++index);
          } else {
            conn.command(`changelevel ${maps[n]}`, 1000)
             .then(() => {
               log.info(`Switching to ${maps[n]}`);
               setTimeout(throttleAttemptScreenshot, 20000);
             })
             .catch((err) => {
               log.warn(`Failed to switch map. Retrying.`);
               setTimeout(() => {
                 switchMap(n);
               }, 7000)
             });
          }
        })
    }),
    new Promise((resolve, reject) => {
      setTimeout(reject, 5000);
    })
  ]).then(() => {}).catch(() => switchMap(n));
}


function checkFileExists(filepath){
  return new Promise((resolve, reject) => {
    fs.access(filepath, fs.F_OK, (error) => {
      resolve(!error);
    });
  });
}

function checkMapExists(n) {
    return new Promise((resolve) => {
        Promise.all([
            checkFileExists(`${game_dir}download/maps/${maps[n]}.bsp`),
            checkFileExists(`${game_dir}maps/${maps[n]}.bsp`)
        ]).then((result) => {
            if (result[0] || result[1])
                resolve(true);
            else resolve(false);
        })
    })
}
