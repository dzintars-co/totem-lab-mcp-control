/*
  TOTEM LAB MCP CONTROL — ARDUINO KODA SKAIDROJUMS
  ================================================

  Prezentācijas kopija ar paskaidrojumiem latviešu valodā.
  Iešūšanai izmanto programmu mapē Projects/totem-mcp/firmware/totem_uno/.

  Aparatūra:
    - TotemDuino v1.9 — Arduino Uno R3 saderīga ATmega328P plate;
    - Totem MiniLab IO Side Panel v1.7.

  Programmas uzdevumi:
    1. Nolasīt 3 slēdžus, 3 pogas, 3 potenciometrus un enkoderi.
    2. Par izmaiņām uzreiz ziņot datoram pa USB Serial.
    3. No datora saņemt RGB LED un releja vadības komandas.
    4. Ar D13 LED parādīt seriālo datu apmaiņu.

  Divi datu virzieni:

    PANELIS → Arduino → USB Serial → Go MCP serveris → bridge → Codex CLI
    Codex → Go MCP serveris → USB Serial → Arduino → RGB/relejs

  Šajā failā iekļautas arī sākotnējā types.h datu struktūras, lai visu
  prezentācijā varētu izlasīt vienuviet. Demo fails nav paredzēts iešūšanai.
*/

#include <Arduino.h>       // Arduino funkcijas: pinMode, analogRead, Serial utt.
#include <avr/pgmspace.h> // PSTR glabā teksta sagataves Flash, taupot 2 KB SRAM.
#include <stdint.h>       // Fiksēta izmēra skaitļu tipi: uint8_t, uint32_t utt.
#include <stdio.h>        // snprintf_P teksta formatēšanai.
#include <stdlib.h>       // abs potenciometru starpības aprēķinam.
#include <string.h>       // strcmp, strncmp un strlen komandu apstrādei.

/*
  DATU STRUKTŪRAS
  ---------------
*/

// Mehānisks kontakts nospiešanas brīdī īslaicīgi "dreb". Debouncer glabā
// nefiltrēto un apstiprināto stāvokli, lai viena nospiešana nekļūtu par vairākām.
struct Debouncer {
  bool raw;           // Tūlītējais elektriskais lasījums.
  bool stable;        // Stāvoklis, kas bijis nemainīgs vismaz DEBOUNCE_MS.
  uint32_t changedAt; // Laiks, kad raw pēdējo reizi mainījās.
};

// Viena pilna paneļa stāvokļa kopija.
struct Snapshot {
  // Biti 0..2: slēdži A/B/C; 3..5: pogas A/B/C; 6: enkodera poga.
  uint8_t bits;
  uint16_t pots[3];         // Potenciometri 0..1023.
  int32_t encoder;          // Enkodera uzkrātais kustības skaitītājs.
  uint8_t red, green, blue, relay; // RGB 0..255; relejs 0 vai 1.
};

// Ziņojumu tipi, kurus Arduino var nosūtīt datoram.
enum FrameKind : uint8_t {
  READY_FRAME, // Plate ir startējusies.
  STATE_FRAME, // Fiziskā paneļa stāvoklis ir mainījies.
  ACK_FRAME,   // Datora komanda izpildīta.
  ERROR_FRAME, // Datora komanda nav derīga.
  GAP_FRAME    // Rinda pārpildījās un daži stāvokļi tika zaudēti.
};

enum ErrorCode : uint8_t {
  BAD_COMMAND,   // Nezināma komanda vai nepareizs lauku skaits.
  BAD_ID,        // ID nav derīgs pozitīvs skaitlis.
  BAD_VALUE,     // RGB vai releja vērtība nav derīga.
  BAD_VERSION,   // Komandā nav atbalstītā V=1.
  LINE_TOO_LONG, // Komanda neietilpst saņemšanas buferī.
  BAD_CHAR       // Komandā ir neatļauta vadības rakstzīme.
};

// Viens nosūtīšanai sagatavots ziņojums.
struct Frame {
  Snapshot state;
  uint32_t seq; // Secības numurs palīdz datoram pamanīt iztrūkumu.
  uint32_t id;  // Komandas ID; GAP gadījumā — zaudēto kadru skaits.
  uint8_t kind;
  uint8_t error;
};

/*
  LAIKI UN JUTĪBA
  ----------------

  Tie ir projekta iestatījumi, nevis ražotāja noteiktas vērtības.
*/

const uint32_t BAUD_RATE = 115200;     // USB Serial ātrums.
const uint16_t DEBOUNCE_MS = 15;       // Kontakta drebēšanas filtrs.
const uint16_t POT_SAMPLE_MS = 20;     // Potenciometrus lasa 50 reizes sekundē.
const uint16_t POT_DELTA = 4;          // Mazākas ADC svārstības uzskata par troksni.
const uint16_t ENCODER_REPORT_MS = 10; // Enkodera ziņošanas intervāls.
const uint16_t LED_BLINK_MS = 40;      // D13 mirgošanas solis.
const uint16_t LED_ACTIVITY_MS = 120;  // Pēc 120 ms klusuma D13 nodziest.
const int8_t ENCODER_DIRECTION = 1;    // Maina uz -1, ja virziens jāapgriež.

/*
  PIESLĒGUMI
  ----------

  Slēdži: A=D7, B=D8, C=D10
  Pogas:  A=D11, B=D12, C=A3
  Enkodera poga: D4
  Potenciometri: A=A0, B=A1, C=A2
  RGB: R=D5, G=D6, B=D9
  Relejs: A4
  Enkodera kanāli: A=D2, B=D3
  Datu aktivitātes LED: D13
*/

// Secība atbilst Snapshot.bits bitiem 0..6.
const uint8_t INPUT_PINS[7] = {7, 8, 10, 11, 12, A3, 4};
const uint8_t POT_PINS[3] = {A0, A1, A2};
const uint8_t RGB_R = 5, RGB_G = 6, RGB_B = 9;
const uint8_t RELAY_PIN = A4, ACTIVITY_PIN = 13;
const uint8_t ENCODER_A = 2, ENCODER_B = 3;

void stateFrame(uint8_t kind, uint32_t id = 0);

/*
  DARBA ATMIŅA
  ------------
*/

// 16 vietu riņķveida rinda ļauj turpināt nolasīt paneli, kamēr iepriekšējie
// ziņojumi vēl tiek sūtīti. Uno atmiņa ir maza, tāpēc rinda nav neierobežota.
const uint8_t QUEUE_SIZE = 16;
Frame queue[QUEUE_SIZE];
uint8_t queueHead = 0, queueCount = 0;

Debouncer inputs[7];
Snapshot current = {}; // Jaunākais pilnais stāvoklis; sākumā viss ir 0.

// volatile vajadzīgs, jo encoderInterrupt() šīs vērtības maina ārpus loop().
volatile uint32_t encoderTicks = 0;
volatile uint8_t encoderPhase = 0;

uint32_t sequence = 0, lostFrames = 0;
uint32_t lastPotSample = 0, lastEncoderReport = 0;

// Viena saņemtā komanda drīkst būt līdz 79 simboliem un nosūtāmā atbilde
// līdz 239 simboliem. Pēdējā vieta paliek C virknes beigu '\0' simbolam.
char receiveBuffer[80];
uint8_t receiveLength = 0;
bool discardLine = false;
uint8_t discardError = LINE_TOO_LONG;

char transmitBuffer[240];
uint16_t transmitLength = 0, transmitPosition = 0;

uint32_t lastActivity = 0, lastLEDToggle = 0;
bool trafficSeen = false, ledOn = false;

/*
  ENKODERS
  --------

  D2 un D3 pārtraukumi šo funkciju izsauc pie katras A vai B signāla malas.
  Tabula no iepriekšējās un jaunās AB fāzes nosaka kustības virzienu.
*/
void encoderInterrupt() {
  static const int8_t transitions[16] = {
    0, -1, 1, 0,
    1, 0, 0, -1,
    -1, 0, 0, 1,
    0, 1, -1, 0
  };

  const uint8_t phase =
      (digitalRead(ENCODER_A) << 1) | digitalRead(ENCODER_B);
  const int8_t delta =
      transitions[(encoderPhase << 2) | phase] * ENCODER_DIRECTION;

  // Viena derīga elektriskā pāreja ir viens skaitījums; tas nav obligāti
  // viens mehānisks enkodera klikšķis.
  encoderTicks += delta;
  encoderPhase = phase;
}

// ATmega328P ir 8 bitu procesors. Uz īsu brīdi izslēdz pārtraukumus, lai
// 32 bitu vērtība kopēšanas vidū netiktu daļēji izmainīta.
int32_t readEncoder() {
  noInterrupts();
  const uint32_t value = encoderTicks;
  interrupts();
  return static_cast<int32_t>(value);
}

/*
  D13 DATU AKTIVITĀTES LED
  -----------------------

  Katra saņemta vai nosūtīta baita laikā activity() atjauno aktivitātes laiku.
  updateLED() mirgo bez delay(), tāpēc pārējā programma netiek apturēta.
*/
void activity() {
  const uint32_t now = millis();
  if (!trafficSeen ||
      static_cast<uint32_t>(now - lastActivity) >= LED_ACTIVITY_MS) {
    ledOn = true;
    digitalWrite(ACTIVITY_PIN, HIGH);
    lastLEDToggle = now;
  }
  trafficSeen = true;
  lastActivity = now;
}

void updateLED() {
  const uint32_t now = millis();
  if (!trafficSeen ||
      static_cast<uint32_t>(now - lastActivity) >= LED_ACTIVITY_MS) {
    ledOn = false;
    digitalWrite(ACTIVITY_PIN, LOW);
  } else if (static_cast<uint32_t>(now - lastLEDToggle) >= LED_BLINK_MS) {
    ledOn = !ledOn;
    digitalWrite(ACTIVITY_PIN, ledOn ? HIGH : LOW);
    lastLEDToggle = now;
  }
}

/*
  RGB UN RELEJS
  -------------

  Paneļa RGB kanāli ir aktīvi ar zemu līmeni. Tāpēc lietotāja 255 jeb pilnu
  spilgtumu Arduino PWM izvada kā 0, bet izslēgtu 0 — kā 255.
*/
void applyOutputs() {
  analogWrite(RGB_R, 255 - current.red);
  analogWrite(RGB_G, 255 - current.green);
  analogWrite(RGB_B, 255 - current.blue);
  digitalWrite(RELAY_PIN, current.relay ? HIGH : LOW);
}

/*
  NOSŪTĀMO ZIŅOJUMU RINDA
  -----------------------
*/
bool enqueue(const Frame &frame) {
  if (queueCount == QUEUE_SIZE) return false;
  queue[(queueHead + queueCount) % QUEUE_SIZE] = frame;
  ++queueCount;
  return true;
}

// STATE un ACK vienmēr satur pilnu stāvokli, nevis tikai mainīto lauku.
void stateFrame(uint8_t kind, uint32_t id) {
  Frame frame = {};
  frame.kind = kind;
  frame.id = id;
  frame.seq = ++sequence;
  frame.state = current;
  if (!enqueue(frame) && lostFrames != UINT32_MAX) ++lostFrames;
}

void errorFrame(uint32_t id, uint8_t code) {
  Frame frame = {};
  frame.kind = ERROR_FRAME;
  frame.id = id;
  frame.error = code;
  // pumpReceive() lasa komandas tikai tad, kad rindā ir brīva vieta.
  enqueue(frame);
}

/*
  KOMANDU PALĪGFUNKCIJAS
  ---------------------
*/

// Pieņem tikai pozitīvu decimālu uint32_t bez pārpildes.
bool parseID(const char *text, uint32_t &value) {
  if (!text || !*text) return false;
  value = 0;
  while (*text) {
    if (*text < '0' || *text > '9') return false;
    const uint8_t digit = *text++ - '0';
    if (value > (UINT32_MAX - digit) / 10) return false;
    value = value * 10 + digit;
  }
  return value != 0;
}

// Pārvērš vienu 0..9, a..f vai A..F simbolu skaitlī 0..15.
int8_t hexDigit(char c) {
  if (c >= '0' && c <= '9') return c - '0';
  if (c >= 'a' && c <= 'f') return c - 'a' + 10;
  if (c >= 'A' && c <= 'F') return c - 'A' + 10;
  return -1;
}

/*
  DATORA KOMANDU APSTRĀDE
  ----------------------

  Atļautie formāti:

    GET;V=1;ID=1
    RGB;V=1;ID=2;VALUE=#FF0000
    RELAY;V=1;ID=3;VALUE=1

  V=1 ir seriālā protokola versija. ID ļauj datoram saprast, kuru komandu
  apstiprina saņemtais ACK. Nederīgus vai liekus laukus noraida.
*/
void command(char *line) {
  char *tokens[5] = {};
  uint8_t count = 0;
  tokens[count++] = line;

  // Semikolus aizstāj ar '\0', sadalot vienu buferi atsevišķās C virknēs.
  for (char *p = line; *p; ++p) {
    if (*p == ';') {
      *p = '\0';
      if (count == 5) {
        errorFrame(0, BAD_COMMAND);
        return;
      }
      tokens[count++] = p + 1;
    }
  }

  uint32_t id = 0;
  if (count < 3 ||
      strncmp(tokens[2], "ID=", 3) ||
      !parseID(tokens[2] + 3, id)) {
    errorFrame(0, BAD_ID);
    return;
  }
  if (strcmp(tokens[1], "V=1")) {
    errorFrame(id, BAD_VERSION);
    return;
  }

  // GET neko nemaina; tas atgriež jaunāko pilno stāvokli.
  if (!strcmp(tokens[0], "GET") && count == 3) {
    current.encoder = readEncoder();
    stateFrame(ACK_FRAME, id);
    return;
  }

  if (count != 4 || strncmp(tokens[3], "VALUE=", 6)) {
    errorFrame(id, BAD_COMMAND);
    return;
  }
  const char *value = tokens[3] + 6;

  if (!strcmp(tokens[0], "RGB")) {
    // Krāsai jābūt #RRGGBB formā ar sešiem heksadecimāliem cipariem.
    if (strlen(value) != 7 || value[0] != '#') {
      errorFrame(id, BAD_VALUE);
      return;
    }
    uint8_t channels[3];
    for (uint8_t i = 0; i < 3; ++i) {
      const int8_t hi = hexDigit(value[1 + 2 * i]),
                   lo = hexDigit(value[2 + 2 * i]);
      if (hi < 0 || lo < 0) {
        errorFrame(id, BAD_VALUE);
        return;
      }
      channels[i] = (hi << 4) | lo;
    }
    current.red = channels[0];
    current.green = channels[1];
    current.blue = channels[2];

  } else if (!strcmp(tokens[0], "RELAY")) {
    if (strcmp(value, "0") && strcmp(value, "1")) {
      errorFrame(id, BAD_VALUE);
      return;
    }
    current.relay = value[0] - '0';

  } else {
    errorFrame(id, BAD_COMMAND);
    return;
  }

  applyOutputs();
  current.encoder = readEncoder();

  // ACK apstiprina Arduino piemēroto programmas vērtību. Tas nav optisks
  // LED spilgtuma vai releja mehāniskā kontakta atgriezeniskais mērījums.
  stateFrame(ACK_FRAME, id);
}

/*
  SERIĀLO DATU SAŅEMŠANA
  ----------------------

  Baitus krāj līdz '\n'. '\r' ignorē, tāpēc der gan LF, gan CRLF rindas.
  Neatļautu vai pārāk garu rindu izlaiž līdz beigām un atbild ar kļūdu.
*/
void pumpReceive() {
  while (Serial.available() && queueCount < QUEUE_SIZE) {
    const char c = Serial.read();
    activity();
    if (c == '\r') continue;

    if (c == '\n') {
      if (discardLine) errorFrame(0, discardError);
      else if (receiveLength) {
        receiveBuffer[receiveLength] = '\0';
        command(receiveBuffer);
      }
      receiveLength = 0;
      discardLine = false;

    } else if (!discardLine) {
      if (c < 32 || c > 126) { discardLine = true; discardError = BAD_CHAR; }
      else if (receiveLength == sizeof(receiveBuffer) - 1) { discardLine = true; discardError = LINE_TOO_LONG; }
      else receiveBuffer[receiveLength++] = c;
    }
  }
}

/*
  ZIŅOJUMA FORMATĒŠANA
  --------------------

  READY paziņo startu. STATE satur pilnu stāvokli pēc starta, izmaiņas vai
  sinhronizācijas. ACK apstiprina komandu. ERR apraksta kļūdu. GAP brīdina,
  ka rindas pārpildes dēļ izlaisti kadri.

  Lauku saīsinājumi: SW=slēdzis, B=poga, P=potenciometrs, ENC=enkoders,
  EB=enkodera poga, RGB=krāsa un RELAY=relejs.
*/
void formatFrame(const Frame &frame) {
  int n = 0;

  if (frame.kind == READY_FRAME) {
    n = snprintf_P(
        transmitBuffer, sizeof(transmitBuffer), PSTR("READY;V=1\n"));

  } else if (frame.kind == GAP_FRAME) {
    n = snprintf_P(
        transmitBuffer, sizeof(transmitBuffer),
        PSTR("GAP;V=1;LOST=%lu\n"),
        static_cast<unsigned long>(frame.id));

  } else if (frame.kind == ERROR_FRAME) {
    const char *codes[] = {
      "BAD_COMMAND", "BAD_ID", "BAD_VALUE",
      "BAD_VERSION", "LINE_TOO_LONG", "BAD_CHAR"
    };
    n = snprintf_P(
        transmitBuffer, sizeof(transmitBuffer),
        PSTR("ERR;V=1;ID=%lu;CODE=%s\n"),
        static_cast<unsigned long>(frame.id), codes[frame.error]);

  } else {
    const Snapshot &s = frame.state;
    const bool ack = frame.kind == ACK_FRAME;
    n = snprintf_P(
        transmitBuffer, sizeof(transmitBuffer),
        PSTR("%s;V=1;SEQ=%lu;SW1=%u;SW2=%u;SW3=%u;B1=%u;B2=%u;B3=%u;P1=%u;P2=%u;P3=%u;ENC=%ld;EB=%u;RGB=#%02X%02X%02X;RELAY=%u"),
        ack ? "ACK" : "STATE", static_cast<unsigned long>(frame.seq),
        (s.bits >> 0) & 1, (s.bits >> 1) & 1, (s.bits >> 2) & 1,
        (s.bits >> 3) & 1, (s.bits >> 4) & 1, (s.bits >> 5) & 1,
        s.pots[0], s.pots[1], s.pots[2], static_cast<long>(s.encoder),
        (s.bits >> 6) & 1, s.red, s.green, s.blue, s.relay);

    if (n > 0 && n < static_cast<int>(sizeof(transmitBuffer))) {
      if (ack) n += snprintf_P(
          transmitBuffer + n, sizeof(transmitBuffer) - n,
          PSTR(";ID=%lu\n"), static_cast<unsigned long>(frame.id));
      else n += snprintf_P(
          transmitBuffer + n, sizeof(transmitBuffer) - n, PSTR("\n"));
    }
  }

  // Ja teksts neietilptu buferī, nenosūta nepilnu protokola rindu.
  transmitLength =
      n > 0 && n < static_cast<int>(sizeof(transmitBuffer)) ? n : 0;
  transmitPosition = 0;
}

/*
  SERIĀLO DATU NOSŪTĪŠANA
  -----------------------

  Atbildi sūta pa daļām atbilstoši brīvajai Serial bufera vietai. Programma
  negaida visas rindas nosūtīšanu un turpina apkalpot fizisko paneli.
*/
void pumpTransmit() {
  if (transmitPosition == transmitLength && queueCount) {
    formatFrame(queue[queueHead]);
    queueHead = (queueHead + 1) % QUEUE_SIZE;
    --queueCount;
  }
  while (transmitPosition < transmitLength &&
         Serial.availableForWrite() > 0) {
    Serial.write(
        static_cast<uint8_t>(transmitBuffer[transmitPosition++]));
    activity();
  }
}

/*
  PANEĻA NOLASĪŠANA
  -----------------

  Digitālajiem kontaktiem LOW nozīmē aktīvu stāvokli. Potenciometru mazās
  svārstības filtrē. Enkodera skaitītāju pārbauda ik pēc 10 ms. Ja kaut kas
  mainījies, izveido vienu STATE ar visu jauno stāvokli.
*/
void scanInputs() {
  const uint32_t now = millis();
  bool changed = false;

  // Slēdži, pogas un enkodera poga.
  for (uint8_t i = 0; i < 7; ++i) {
    const bool pressed = digitalRead(INPUT_PINS[i]) == LOW;
    if (pressed != inputs[i].raw) {
      inputs[i].raw = pressed;
      inputs[i].changedAt = now;
    }
    if (pressed != inputs[i].stable &&
        static_cast<uint32_t>(now - inputs[i].changedAt) >= DEBOUNCE_MS) {
      inputs[i].stable = pressed;
      if (pressed) current.bits |= 1 << i; else current.bits &= ~(1 << i);
      changed = true;
    }
  }

  // Potenciometri 0..1023. Galapunktus 0 un 1023 pieņem arī pie mazas
  // starpības, lai būtu iespējami precīzi 0% un 100%.
  if (static_cast<uint32_t>(now - lastPotSample) >= POT_SAMPLE_MS) {
    lastPotSample = now;
    for (uint8_t i = 0; i < 3; ++i) {
      const uint16_t sample = analogRead(POT_PINS[i]);
      const int difference =
          static_cast<int>(sample) - static_cast<int>(current.pots[i]);
      if (abs(difference) >= POT_DELTA ||
          ((sample == 0 || sample == 1023) && sample != current.pots[i])) {
        current.pots[i] = sample; changed = true;
      }
    }
  }

  if (static_cast<uint32_t>(now - lastEncoderReport) >= ENCODER_REPORT_MS) {
    lastEncoderReport = now;
    const int32_t value = readEncoder();
    if (value != current.encoder) { current.encoder = value; changed = true; }
  }

  if (changed) stateFrame(STATE_FRAME);
}

/*
  SETUP — IZPILDĀS VIENU REIZI
  ----------------------------

  Vispirms uzstāda drošus izeju līmeņus, pēc tam sagatavo ieejas, enkodera
  pārtraukumus un Serial. Beigās rindā ieliek READY un sākuma STATE.
*/
void setup() {
  // Relejs startā izslēgts.
  digitalWrite(RELAY_PIN, LOW);
  pinMode(RELAY_PIN, OUTPUT);

  // Aktīvi-zemais RGB startā izslēgts.
  digitalWrite(RGB_R, HIGH);
  digitalWrite(RGB_G, HIGH);
  digitalWrite(RGB_B, HIGH);
  pinMode(RGB_R, OUTPUT);
  pinMode(RGB_G, OUTPUT);
  pinMode(RGB_B, OUTPUT);

  digitalWrite(ACTIVITY_PIN, LOW);
  pinMode(ACTIVITY_PIN, OUTPUT);

  for (uint8_t i = 0; i < 7; ++i) {
    // Pirmajām sešām ieejām izmanto iekšējo pull-up, bet
    // enkodera pogai D4 — parastu INPUT atbilstoši paneļa pieslēgumam.
    pinMode(INPUT_PINS[i], i == 6 ? INPUT : INPUT_PULLUP);
    inputs[i].raw =
        inputs[i].stable = digitalRead(INPUT_PINS[i]) == LOW;
    if (inputs[i].stable) current.bits |= 1 << i;
  }

  for (uint8_t i = 0; i < 3; ++i) current.pots[i] = analogRead(POT_PINS[i]);

  pinMode(ENCODER_A, INPUT);
  pinMode(ENCODER_B, INPUT);
  encoderPhase =
      (digitalRead(ENCODER_A) << 1) | digitalRead(ENCODER_B);
  attachInterrupt(
      digitalPinToInterrupt(ENCODER_A), encoderInterrupt, CHANGE);
  attachInterrupt(
      digitalPinToInterrupt(ENCODER_B), encoderInterrupt, CHANGE);

  applyOutputs();
  Serial.begin(BAUD_RATE);

  Frame ready = {};
  ready.kind = READY_FRAME;
  enqueue(ready);
  stateFrame(STATE_FRAME);
}

/*
  LOOP — ATKĀRTOJAS NEPĀRTRAUKTI
  -----------------------------

  Nav delay(), tāpēc sūtīšana, nolasīšana, komandu saņemšana un LED
  indikācija regulāri saņem izpildes laiku.
*/
void loop() {
  pumpTransmit(); // Turpina nosūtīt rindas vecāko ziņojumu.
  scanInputs();   // Nolasa fizisko paneli.

  // Pēc rindas pārpildes nosūta GAP un jaunu pilnu STATE, lai dators varētu
  // atjaunot pareizu stāvokli arī tad, ja starpstāvokļi tika zaudēti.
  if (lostFrames && queueCount <= QUEUE_SIZE - 2) {
    Frame gap = {};
    gap.kind = GAP_FRAME;
    gap.id = lostFrames;
    lostFrames = 0;
    enqueue(gap);
    stateFrame(STATE_FRAME);
  }

  pumpReceive();  // Pieņem GET, RGB un RELAY komandas.
  pumpTransmit(); // Ļauj tikko izveidotu ACK sākt sūtīt uzreiz.
  updateLED();    // Atjaunina D13 bez programmas aizturēšanas.
}

/*
  ĪSS PIEMĒRS
  ------------

  Dators nosūta:

    RGB;V=1;ID=25;VALUE=#FF0000

  Arduino pārbauda komandu, ieslēdz sarkano kanālu un atbild:

    ACK;V=1;SEQ=42;SW1=0;SW2=0;SW3=0;B1=0;B2=0;B3=0;
    P1=427;P2=194;P3=849;ENC=0;EB=0;RGB=#FF0000;RELAY=0;ID=25

  Šeit garais ACK pārskatāmībai parādīts vairākās rindās. Pa USB tas ir viens
  ziņojums vienā rindā līdz noslēdzošajam '\n'.

  Ja cilvēks nospiež paneļa pogu, Arduino pēc 15 ms filtra pats nosūta
  STATE. Datoram nav periodiski jājautā, vai fiziskais panelis mainījies.

  Visa būtība vienā teikumā:
  Arduino tieši apkalpo aparatūru, bet Go MCP serveris šo seriālo protokolu
  pārvērš Codex saprotamos stāvokļos, notikumos un vadības rīkos.
*/

// =========================== FAILA BEIGAS ===========================
