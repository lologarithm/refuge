var xmlns = "http://www.w3.org/2000/svg";

// global websocket
var ws;

// global editing flag, used to disable controls and allow moving them around and attaching to rooms.
var editing = false;

// use to show ! when discon
var connErr = document.getElementById("conn");

// Mostly used for resizing window.
var floor1 = document.getElementById("floor1");
var floor2 = document.getElementById("floor2");
var maincanvas = document.getElementById("maincanvas");
var inter = document.getElementById("interaction");

// debugging handlers
// maincanvas.addEventListener("mousemove", function(e) {
//   var svgp = svgPoint(maincanvas, e.clientX, e.clientY)
//   console.log("SVG Point:", svgp);
// });
var header = document.getElementById("header");

// doSize handles window resizing --
// allow changing between portrait and landscape.
var doSize = function() {
  if (header == null) {
    header = document.getElementById("header");
    window.setTimeout(doSize, 10);
    return;
  }
  var headerHeight = header.getBoundingClientRect().height;
  inter.style.top = (headerHeight-25)+"px";
  var w = window.innerWidth-25;
  if (window.innerWidth < window.innerHeight) {
    maincanvas.setAttribute("width", w);
    maincanvas.setAttribute("height", (w)*2);
    maincanvas.setAttribute("viewBox", "0 0 600 1200");
    floor2.setAttribute("transform", "translate(0,600)");
  } else {
    var maxHeight = (window.innerHeight-headerHeight);
    maincanvas.setAttribute("width", w);
    maincanvas.setAttribute("height", "100%");
    maincanvas.setAttribute("viewBox", "0 0 1200 600");
    floor2.setAttribute("transform", "translate(600, 0)");
  }
};

var graphs = document.getElementById("graphs");
var dt = document.getElementById("dt");
var weather = document.getElementById("weather");

var cmaxed = false;
graphs.addEventListener("click", function(){
  if (!cmaxed) {
    graphs.style.height = "100vh";
    graphs.style.width = "100vw";
    cmaxed = true;
  } else {
    graphs.style.height = "200px";
    graphs.style.width = "50%";
    cmaxed = false;
  }
  getStats(graphs);
});
var doGraphs = function() {
  window.setTimeout(doGraphs, 60000);
  getStats(graphs);
}
var doTime = function() {
  window.setTimeout(doTime, 1000);
  dt.innerText = getTime();
}
var doWeather = function() {
  window.setTimeout(doWeather, 60000);
  getWeather(weather);
}
doGraphs();
doWeather();
doTime();

window.addEventListener("resize", doSize)
doSize();
var toggle = document.getElementById("unittoggle");
toggle.addEventListener("click", toggleUnits);

var units = getUnits();
if (units == "F") {
    toggle.innerText = "Fahrenheit";
} else {
    toggle.innerText = "Celcius";
}

var editToggle = document.getElementById("edittoggle");
editToggle.addEventListener("click", function() {
  editing = !editing;
});


// Global list of all devices we are rendering.
var devices = {};

console.log("Starting websocket...")
onClose(); // Reconnects the web socket

// websocket functions
function onOpen(event) {
  console.log("Socket opened.");
  conn.innerText = "";
}
function onMessage(event) {
  var msg = JSON.parse(event.data);
  console.log("Msg: ", msg);
  updateDevice(msg);
}
function onClose(event) {
  conn.innerText = "!";
  if (event) {
    console.log("Closed event: ", event);
  }
  var timeout = 0;
  if (ws) {
    console.log("Reconnecting...")
    timeout = 1000;
  }
  setTimeout(function(){
    var prot = "wss://"
    if (location.protocol != 'https:') {
      prot = "ws://"
    }
    ws = new WebSocket(prot + location.host + "/stream");
    ws.addEventListener('close', onClose)
    ws.addEventListener('open', onOpen)
    ws.addEventListener('message', onMessage);
  }, timeout)
}

// updateDevice is called on a message from network.
function updateDevice(msg) {
  var prefix = "td";
  if (msg.Portal != null) {
    prefix = "pt";
  } else if (msg.Switch != null) {
    prefix = "sw"
  }
  var id = prefix + msg.Name.replace(/\s/g, "");
  var device = devices[id];

  if (device == null || device == undefined) {
    // Create base device
    device = createDevice(id, msg);

    // Add device type specific additions.
    if (prefix == "td") {
      createThermostat(device);
    } else if (prefix =="pt") {
      createPortal(device);
    } else if (prefix == "sw") {
      createSwitch(device);
    }
  }
  // Now cause device to re-render with new data.
  device.update(msg);
  device.msg = msg;
}

// createDevice is generic function to create a new device.
function createDevice(id, msg) {
  var tmpl = "thermoTemplate";
  if (msg.Portal != null) {
    tmpl = "portalTemplate";
  } else if (msg.Switch != null) {
    tmpl = "switchTemplate";
  }
  var itemEle = document.getElementById(tmpl).cloneNode(true);
  itemEle.id = id;
  itemEle.childNodes[0].textContent = msg.Name;
  maincanvas.appendChild(itemEle);

  device = {};
  if (msg.Pos.RoomID != "") {
    var attached = document.getElementById(msg.Pos.RoomID);
    device.attached = attached;
    itemEle.setAttribute("transform", "translate(" + msg.Pos.X + "," + msg.Pos.Y + ") scale(1.25)")
    attached.parentElement.parentElement.appendChild(itemEle);
  }
  device.itemEle = itemEle;
  device.name = msg.Name;
  device.msg = msg;
  devices[id] = device;
  addEditorControls(device);
  return device;
}

// createX functions create the control specific dom elements and interaction handlers.
function createThermostat(device) {
  var thermoControl = null;
  for (var i = 0; i < device.itemEle.childNodes.length; i++) {
    var ch = device.itemEle.childNodes[i];
    if (ch.tagName != "g") {
      continue;
    }
    var cl = ch.getAttribute("class");
    if (cl == "controls") {
      thermoControl = ch;
      break;
    }
  }
  thermoControl.style.display = "none";
  thermoControl.style.position = "absolute";
  device.touching = false;
  device.thermoControl = thermoControl;
  var contVis = false;
  device.itemEle.childNodes[4].addEventListener("click", function(e) {
    if (editing) {
      return;
    }
    if (contVis) {
      device.thermoControl.style.display = "none";
      contVis = false;
    } else {
      animateThermoOpen(device);
      contVis = true;
    }
    e.preventDefault();
  });
  thermoInteraction(device); // Add thermo controls UI

  var devdom = device.itemEle.childNodes[4];
  device.update = function(msg) {
    if (!device.touching) {
      drawThermoLines(device.thermoControl, msg.Thermostat.Settings.High, msg.Thermostat.Settings.Low, msg.Thermometer.Temp, thermStatus.committed);
    }
    var temp = msg.Thermometer.Temp;
    // var hum = msg.Thermometer.Humidity; // not used
    // var target = msg.Thermostat.Target;
    if (units == "F") {
      // target = convertToF(target);
      temp = convertToF(temp);
    }
    // Now update the values for current temp/hum and target.
    // thdiv.childNodes[2].childNodes[1].innerText = thdata.Humidity;

    devdom.childNodes[3].textContent = temp.toFixed(0) + "*";

    if (msg.Thermostat.State == 1) { // Cooling
      devdom.childNodes[1].setAttribute("fill", "#3399FF");
      devdom.childNodes[1].classList.add("anicircle");
    } else if (msg.Thermostat.State == 2) { // Fan
      devdom.childNodes[1].setAttribute("fill", "#CCCCCC");
      devdom.childNodes[1].classList.add("anicircle");
    } else if (msg.Thermostat.State == 3) { // Heating
      devdom.childNodes[1].setAttribute("fill", "#FF6666");
      devdom.childNodes[1].classList.add("anicircle");
    } else {
      devdom.childNodes[1].classList.remove("anicircle");
      devdom.childNodes[1].setAttribute("fill", "#999999");
      devdom.childNodes[1].setAttribute("fill-opacity", "1.0");
    }
  }
}
function createPortal(device) {
  device.itemEle.addEventListener("click", function(){
    var state = 1;
    if (device.msg.Portal.State == 1) {
      state = 2;
    }
    var msg = JSON.stringify({Name: device.name, Toggle: state})
    ws.send(msg);
    console.log(msg);
  });
  device.update = function(msg) {
    device.itemEle.childNodes[2].style.display = "none";
    device.itemEle.childNodes[4].style.display = "none";
    device.itemEle.childNodes[6].style.display = "none";
    device.itemEle.childNodes[8].style.display = "none";
    if (device.msg.Portal.State != 1) {
      // Open state
      if (device.hovered) {
        device.itemEle.childNodes[4].style.display = "block";
      } else {
        device.itemEle.childNodes[2].style.display = "block";
      }
    } else {
      // Closed State
      if (device.hovered) {
        device.itemEle.childNodes[8].style.display = "block";
      } else {
        device.itemEle.childNodes[6].style.display = "block";
      }
    }
  }
  device.itemEle.addEventListener("mouseover", function(e) {
    device.hovered = true;
    device.update();
  });
  device.itemEle.addEventListener("mouseout", function(e) {
    device.hovered = false;
    device.update();
  });
}
function createSwitch(device) {
  device.itemEle.addEventListener("click", function() {
    if (editing) {
      return;
    }
    var on = 1;
    if (device.msg.Switch.On) {
      on = 2
    }
    var msg = JSON.stringify({Name: device.name, Toggle: on})
    ws.send(msg);
    console.log(msg);
  });
  device.update = function(msg) {
    device.msg = msg;
    if (msg.Switch.On) {
      device.itemEle.childNodes[4].setAttribute("fill", "url('#fire')");
    } else {
      device.itemEle.childNodes[4].setAttribute("fill", "black");
    }
  }
}

// Animates the thermostat dial spinning out.
function animateThermoOpen(device) {
  device.itemEle.style.backgroundColor = "gray";
  device.thermoControl.style.display = "block";
  // Start the range markers hidden
  device.thermoControl.childNodes[5].style.display = "none";
  device.thermoControl.childNodes[7].style.display = "none";
  var animateAngle = function(step) {
    if (step > 11) {
      device.thermoControl.childNodes[5].style.display = "block";
      device.thermoControl.childNodes[7].style.display = "block";
      return;
    }

    var start = -Math.PI/2;
    var stepA = Math.PI/12;
    var x = Math.cos(start+stepA*(step+1));
    var y = Math.sin(start+stepA*(step+1));
    if (step < 6) {
      device.thermoControl.childNodes[1].setAttribute("d", "M 0 50 L 0 0 A 100 100 0 0 1 " + (x * 100) + " " + (100+(y*100)) + " L " + (x*50) + " " + (100+(y*50)) + " A 50 50 0 0 0 0 50");
    } else {
      device.thermoControl.childNodes[3].setAttribute("d", "M 50 100 L 100 100 A 100 100 0 0 1 " + (x * 100) + " " + (100+(y*100)) + " L " + (x*50) + " " + (100+(y*50)) + " A 50 50 0 0 0 50 100");
    }
    window.requestAnimationFrame(function(){ animateAngle(step+1)});
  }
  device.thermoControl.childNodes[3].setAttribute("d", "");
  animateAngle(0);
}

window.addEventListener("touchmove", function(e){
  // console.log("Window touchmove?", e);
  // This is mostly currently for debugging.
  // Also, it looks like chrome fires more touchmove events on the svg
  // elements if this event is registered (why chrome, why?)
});

// thermoInteraction adds thermostat control interactions to the given device.
// device.thermoControl must already be created.
function thermoInteraction(device) {
  // Setup thermostat control UI
  var grabbing = 0;
  var angle = 0;
  var spread = device.msg.Thermostat.Settings.High - device.msg.Thermostat.Settings.Low;
  var mid = (device.msg.Thermostat.Settings.High + device.msg.Thermostat.Settings.Low)/2;
  var ibr = device.itemEle.getBoundingClientRect();
  var isp = svgPoint(maincanvas, ibr.x, ibr.y);
  var center = {x: isp.x+40, y: isp.y+60};

  var touches = [];
  var touchRange = 0;

  var getTouch = function(id) {
    for (var j = 0; j < touches.length; j++) {
      if (touches[j].t.identifier == id) {
        return touches[j];
      }
    }
    return null;
  }
  var handleMove = function(x, y) {
    var nangle = calcAngle(x,y);
    var adiff = angle - nangle;
    var frac = adiff / Math.PI;
    var m = mid + (frac*30);
    var l = m - spread/2;
    var h = m + spread/2;
    if (m < 15 || m > 35 || spread > 25) {
      return; // prevent stupid mistakes
    }
    drawThermoLines(device.thermoControl, h, l, device.msg.Thermometer.Temp, thermStatus.setting);
  }
  var calcAngle = function(x,y) {
    var esp = svgPoint(maincanvas, x, y);
    var vec = {x: center.x - esp.x, y: center.y - esp.y}
    var angle = Math.atan2(vec.y, vec.x) - Math.atan2(10, 0);
    var twopi = Math.PI * 2;
    if (angle < -Math.PI) {
      angle += Math.PI*2;
    }
    if (angle > Math.PI) {
      angle -= Math.PI*2;
    }
    return angle;
  }
  var sendThermoUpdate = function(m) {
    if (m < 15 || m > 35 || spread > 25) {
      return; // prevent stupid mistakes
    }
    device.msg.Thermostat.Settings.Low =  m - spread/2;
    device.msg.Thermostat.Settings.High = m + spread/2;
    drawThermoLines(device.thermoControl, device.msg.Thermostat.Settings.High, device.msg.Thermostat.Settings.Low, device.msg.Thermometer.Temp, thermStatus.pushed);
    var msg = JSON.stringify({Name: device.name, Climate: {High: m + spread/2, Low: m - spread/2}} );
    ws.send(msg);
    console.log("sent: ", msg);
  }
  device.thermoControl.addEventListener("mousedown", function(e){
    grabbing = 1;
    device.touching = true;
    angle = calcAngle(e.clientX, e.clientY)
    e.preventDefault();
  }, {passive: false})
  window.addEventListener("mousemove", function(e) {
    if (grabbing == 0) {
      return;
    }
    handleMove(e.clientX, e.clientY);
    e.preventDefault();
  })
  // var dbgr = document.getElementById("debugger");
  window.addEventListener("mouseup", function(e){
    if (grabbing == 0) {
      return;
    }
    device.touching = false;
    grabbing = 0;
    var nangle = calcAngle(e.clientX, e.clientY)
    var adiff = angle - nangle;
    var frac = adiff / Math.PI;

    m = mid + (frac*30);
    if (m < 15 || m > 35) {
      return;
    }
    sendThermoUpdate(m);
    e.preventDefault();
  }, {passive: false})
  device.thermoControl.addEventListener("touchstart", function(e){
    e.preventDefault();
    if (!e.cancelable) {
      console.log("Not able to cancel this event: ", e);
      return;
    }
    device.touching = true;
    for (var i = 0; i < e.changedTouches.length; i++) {
        touches.push({
          t: copyTouch(e.changedTouches[i]),
          a: calcAngle(e.changedTouches[i].clientX, e.changedTouches[i].clientY)
        });
    }
    if (touches.length == 1) {
      angle = calcAngle(touches[0].t.clientX, touches[0].t.clientY);
      if (e.cancelable) {
        e.preventDefault();
      }
    }
    if (touches.length == 2) {
      var a = touches[0].t.clientX-touches[1].t.clientX;
      var b = touches[0].t.clientY-touches[1].t.clientY;
      touchRange = a*a + b*b;
      if (e.cancelable) {
        e.preventDefault();
      }
    }
  }, {capture: true, passive: false})
  device.thermoControl.addEventListener("touchend", function(e){
    if (touches.length == 0) {
      return;
    }
    var m = mid;
    if (touches.length == 1) {
      var nangle = calcAngle(touches[0].t.clientX, touches[0].t.clientY)
      var adiff = angle - nangle;
      var frac = adiff / Math.PI;
      m = mid + (frac*30);
    }
    for (var i = 0; i < e.changedTouches.length; i++) {
      var cht = e.changedTouches[i];
      touches = touches.filter(function(v){
        if (v.t.identifier == cht.identifier) {
          e.preventDefault();
          return false;
        }
        return true;
      })
    }
    device.touching = touches.length > 0;
    sendThermoUpdate(m);
    e.preventDefault();
  }, {passive: false})
  device.thermoControl.addEventListener("touchmove", function(e){
    // console.log("TouchMove: ", touches);
    if (touches.length == 0) {
      return;
    }
    var ch = false;
    for (var i = 0; i < e.changedTouches.length; i++) {
      var chTch = e.changedTouches[i];
      var touch = getTouch(chTch.identifier);
      if (touch != null) {
        touch.t = copyTouch(chTch);
        e.preventDefault();
        ch = true;
      }
    }
    if (!ch) {
      return;
    }
    if (touches.length == 1) {
      handleMove(touches[0].t.clientX, touches[0].t.clientY);
    } else if (touches.length == 2) {
      var a = touches[0].t.clientX-touches[1].t.clientX;
      var b = touches[0].t.clientY-touches[1].t.clientY;
      var newTouchRange = a*a + b*b;
      var ratio  = newTouchRange / touchRange;
      spread = 10*(ratio/2);
      if (spread > 15) {
        spread = 15;
      } else if (spread < 5) {
        spread = 5;
      }
      var l = mid - spread/2;
      var h = mid + spread/2;
      drawThermoLines(device.thermoControl, h, l, device.msg.Thermometer.Temp, thermStatus.setting);
    }
  }, {passive: false})
  device.thermoControl.addEventListener("touchcancel", function(e){
    console.log("Cancelled:", e.changedTouches);
  });
}

// getRoomPos returns the SVG coords of the room (offset from the floor)
// This lets you attach devices to the floor instead of the room (helps with layering)
function getRoomPos(left, top) {
  var overEle = document.elementsFromPoint(left, top);
  if (overEle != null) {
    for (var i = 0; i < overEle.length; i++) {
      var p = overEle[i].parentElement;
      if (p == null || p == undefined) {
        continue
      }
      for (var ci = 0; ci < p.classList.length; ci++) {
        if (p.classList[ci] == "room") {
          var pbr = p.parentElement.parentElement.getBoundingClientRect();
          var pp = svgPoint(maincanvas, pbr.x, pbr.y);
          var mp = svgPoint(maincanvas, left, top);
          return {id: p.id, x: Math.floor(mp.x-pp.x), y: Math.floor(mp.y-pp.y)};
        }
      }
    }
  }
  return null;
}

// returns the SVG coordinate space point of the given dom x/y position
function svgPoint(element, x, y) {
  var pt = maincanvas.createSVGPoint();
  pt.x = x;
  pt.y = y;
  return pt.matrixTransform(element.getScreenCTM().inverse());
}

// addEditorControls will make devices able to be dragged/dropped into rooms.
// when editing is active all interactions (changing temp, toggling switches, etc) are disabled.
function addEditorControls(device) {
  var mousedown = 0;
  var offx = 0;
  var offy = 0;
  var left = 0;
  var top = 0;
  var touching = false;
  var thermoVis = false;

  var highlighted = null;

  // Setup
  var moveControl = function(x, y) {
    var svgp = svgPoint(maincanvas, x, y);
    left = svgp.x-offx;
    top = svgp.y-offy;
    device.itemEle.setAttribute("transform", "translate(" + left + "," + top + ") scale(1.25)")

    var roomPos = getRoomPos(x, y);
    if (roomPos != null) {
      if (highlighted == null || roomPos.id != highlighted.id) {
        if (highlighted != null) {
          highlighted.childNodes[1].setAttribute("fill-opacity", "0.0")
        }
        var item = document.getElementById(roomPos.id);
        if (item != null) {
          item.childNodes[1].setAttribute("fill", "red")
          item.childNodes[1].setAttribute("fill-opacity", "0.3")
          highlighted = item;
        }
      }
    } else {
      if (highlighted != null) {
        highlighted.childNodes[1].setAttribute("fill-opacity", "0.0")
        highlighted = null;
      }
    }
  }
  device.itemEle.addEventListener("mousedown", function(e){
    if (touching || !editing) {
      return;
    }
    mousedown = 1;
    var brect = device.itemEle.getBoundingClientRect();
    var srect = svgPoint(maincanvas, brect.x, brect.y);
    var off = svgPoint(maincanvas, e.clientX, e.clientY);
    console.log("mousedown, off, brect", off, brect, srect);
    offx = off.x-srect.x;
    offy = off.y-srect.y;
    maincanvas.appendChild(device.itemEle);
    device.itemEle.setAttribute("transform", "translate(" + srect.x + "," + srect.y + ")");
    e.preventDefault();
  });
  window.addEventListener("mouseup", function(e){
    if (touching || !editing || mousedown == 0) {
      return;
    }
    var roomPos = getRoomPos(e.clientX, e.clientY);
    if (roomPos != null) {
      // TODO: re-attach to correct floor.
      var msg = JSON.stringify({Name: device.name, Pos: {RoomID: roomPos.id, X: roomPos.x, Y: roomPos.y }});
      ws.send(msg);
      console.log("sent: ", msg);
    }
    mousedown = 0;
  });
  window.addEventListener("mousemove", function(e){
    if (!editing || mousedown == 0 || touching) {
      return;
    }
    // console.log("Mouse move:", e);
    // var off = svgPoint(device.itemEle, e.clientX, e.clientY);
    moveControl(e.clientX, e.clientY);

    e.preventDefault();
  });

  device.itemEle.addEventListener("touchstart", function(e) {
    if (!editing) {
      return;
    }
    touching = true;
    if (!e.cancelable) {
      console.log("Can't cancel this touch event...");
    }

    var brect = device.itemEle.getBoundingClientRect();
    var srect = svgPoint(maincanvas, brect.x, brect.y);
    var off = svgPoint(maincanvas, e.touches[0].clientX, e.touches[0].clientY);
    console.log("mousedown, off, brect", off, brect, srect);
    offx = off.x-srect.x;
    offy = off.y-srect.y;
    maincanvas.appendChild(device.itemEle);
    device.itemEle.setAttribute("transform", "translate(" + srect.x + "," + srect.y + ")")
  }, false);
  device.itemEle.addEventListener("touchend", function(e) {
    if (!editing || !touching) {
      return;
    }
    var roomPos = getRoomPos(e.changedTouches[0].clientX, e.changedTouches[0].clientY);
    if (roomPos != null) {
      // TODO: re-attach to correct floor.
      var msg = JSON.stringify({Name: device.name, Pos: {RoomID: roomPos.id, X: roomPos.x, Y: roomPos.y }});
      ws.send(msg);
      console.log("sent: ", msg);
    }
    touching = false;
  }, false);
  device.itemEle.addEventListener("touchcancel", function(e) {}, false);
  device.itemEle.addEventListener("touchmove", function(e) {
    if (!editing || !touching) {
      return;
    }
    moveControl(e.changedTouches[0].clientX, e.changedTouches[0].clientY);
    e.preventDefault();
  }, false);
}

// status enum
var thermStatus = {
  committed: 0,
  setting: 1,
  pushed: 2,
};
// drawThermoLines will draw the lines and highlighted area for the radial thermostat.
// This will toggle the color of the range based on status (editing, pushed to server, or committed)
function drawThermoLines(thermoControl, high, low, temp, status) {
  // var hV = (high+low)/2;
  var hV = high;
  var lV = low;
  if (units == "F") {
    hV = convertToF(hV);
    lV = convertToF(lV);
  }

  var ca = Math.PI/2 - (((high-10)/30) * Math.PI);
  var x = Math.cos(ca);
  var y = Math.sin(ca);
  thermoControl.childNodes[5].setAttribute("d", "M " + (x * 100) + " " + (100+(y*100)) + "L " + (x*50) + " " + (100+(y*50)));
  thermoControl.childNodes[13].textContent = (hV).toFixed(0);
  thermoControl.childNodes[13].setAttribute("x", x*120);
  thermoControl.childNodes[13].setAttribute("y", 100+(y*120));

  var ca = Math.PI/2 - (((low-10)/30) * Math.PI);
  var xl = Math.cos(ca);
  var yl = Math.sin(ca);
  thermoControl.childNodes[7].setAttribute("d", "M " + (xl * 100) + " " + (100+(yl*100)) + "L " + (xl*50) + " " + (100+(yl*50)));
  thermoControl.childNodes[15].textContent = (lV).toFixed(0);
  thermoControl.childNodes[15].setAttribute("x", xl*120);
  thermoControl.childNodes[15].setAttribute("y", 100+(yl*120));

  var ca = Math.PI/2 - (((temp-10)/30) * Math.PI);
  var xt = Math.cos(ca);
  var yt = Math.sin(ca);
  thermoControl.childNodes[9].setAttribute("d", "M " + (xt * 110) + " " + (100+(yt*110)) + "L " + (xt*40) + " " + (100+(yt*40)));
  // Highlighted area
  thermoControl.childNodes[11].setAttribute("d", "M " + x*50 + " " + (100+(y*50)) + " L " + x*100 + " " + (100+(y*100)) + " A 100 100 0 0 1 " + (xl * 100) + " " + (100+(yl*100)) + " L " + (xl*50) + " " + (100+(yl*50)) + " A 50 50 0 0 0 "  + x*50 + " " + (100+(y*50)));
  if (status == thermStatus.pushed) {
    thermoControl.childNodes[11].setAttribute("fill-opacity", 0.5);
    thermoControl.childNodes[11].setAttribute("fill", "black");
  } else if (status == thermStatus.setting) {
    thermoControl.childNodes[11].setAttribute("fill-opacity", 0.2);
    thermoControl.childNodes[11].setAttribute("fill", "yellow");
  } else {
    thermoControl.childNodes[11].setAttribute("fill-opacity", 0.3);
    thermoControl.childNodes[11].setAttribute("fill", "yellow");
  }
}

// toggleUnits changes the local storage temp units to be used (C/F)
function toggleUnits() {
  var toggle = document.getElementById("unittoggle");

  if (units == "F") {
    localStorage.setItem("units" , "C");
    toggle.innerText = "Celcius";
    units = "C";
  } else {
    localStorage.setItem("units" , "F");
    toggle.innerText = "Fahrenheit";
    units = "F";
  }
}

// getUnits will return the currently stored units (C/F). Defaults to C
function getUnits() {
  var lunits = localStorage.getItem("units");
  if (lunits == null) {
    lunits = "C";
  }
  return lunits;
}

function convertToF(v) {
  return (v*1.8) + 32;
}
function convertToC(v) {
  return (v-32) / 1.8;
}

// copyTouch is a touch helper to clone touch events (some browsers re-use the same touch objects)
function copyTouch(touch) {
  return { identifier: touch.identifier, clientX: touch.clientX, clientY: touch.clientY };
}

function getWeather(domele) {
  // http://wttr.in/?format=4
  var xmlhttp = new XMLHttpRequest();
  xmlhttp.onreadystatechange = function() {
      if (xmlhttp.readyState == XMLHttpRequest.DONE) {   // XMLHttpRequest.DONE == 4
         if (xmlhttp.status == 200) {
             domele.innerHTML = xmlhttp.responseText;
         } else {
           console.log("Failed to fetch weather: ", xmlhttp);
         }
      }
  };
  xmlhttp.open("GET", "/weather", true);
  xmlhttp.send();
}

function getStats(domele) {
  var xmlhttp = new XMLHttpRequest();
  xmlhttp.onreadystatechange = function() {
      if (xmlhttp.readyState == XMLHttpRequest.DONE) {   // XMLHttpRequest.DONE == 4
         if (xmlhttp.status == 200) {
             var data = JSON.parse(xmlhttp.responseText);
             console.log(data);
             var serLook = {};
             var labels = [];
             var series = [];
             for (var i = 0; i < data.length; i++) {
               var name = data[i].Name;
               var lu = serLook[name];
               if (lu == undefined) {
                 lu = labels.length;
                 serLook[name] = lu;
                 labels.push(name);
                 series.push([]);
               }
               series[lu].push({x: Date.parse(data[i].Time), y: data[i].Temp});
             }

             // We are setting a few options for our chart and override the defaults
             var options = {
               high: 28,
               low: 16,
               axisX: {
                 showGrid: false,
                 showLabel: true
               },
               axisY: {
               }
             };
             var c = new Chartist.Line('#graphs', {
               "labels": [],
               "series": series,
             }, options);
         } else {
           console.log("Failed to fetch weather: ", xmlhttp);
         }
      }
  };
  xmlhttp.open("GET", "/stats", true);
  xmlhttp.send();
}

function getTime() {
  var d = new Date();
  return d.toLocaleDateString("en-US", { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' }) + " " + d.toLocaleTimeString();
}
