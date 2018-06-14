package main

var page = `<html>
<head>
  <style>
  div {
    margin: auto;
  }
  form {
    margin: auto;
    width: 650px;
  }
  button {
      height:150px;
      width:150px;
      font-size: 6em;
  }
  #goalC {
    font-size: 5em;
    margin-left: 50px;
    width: 100px;
    height: 150px;
    color: white;
    background-color: transparent;
    border: none;
  }
  #downC {
    margin-left:50px;
  }
  </style>
</head>
<body style="background-color: black;color: white">
  <p style="font-size: 4em;">%s</p>
  <div style="width: 610px">
    <p style="font-size: 6em;">%.1fC / %dF<br />%.1f%%</p>
  </div>
  <div style="">
    <form action="/set" method="post">
      <button action="submit" name="upc" id="upC">▲</button>
      <input type="text" id="goalC" name="goalc" value="%d" />
      <button action="submit" name="downc" id="downC">▼</button>
    </form>
  </div>
</body>
</html>`
