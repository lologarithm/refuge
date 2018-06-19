package main

var page = `<html>
<head>
  <script>
    window.onload=function(){
      var f = document.getElementById("refresh");
      window.setTimeout(function() {f.submit();}, 5000);
    };
  </script>
  <style>
  div {
    margin: auto;
  }
  form {
    margin: auto;
    width: 40vw;
  }
  button {
      height:15vw;
      width:15vw;
      font-size: 6vw;
  }
  #goalC {
    font-size: 8vw;
    width: 15vw;
    height: 15vw;
    color: white;
    background-color: transparent;
    border: none;
    margin-left: 10vw;
  }
  #downC {
    margin-left:5vw;
  }
  </style>
</head>
<body style="background-color: black;color: white">
  <p style="font-size: 4vw;">%s</p>
  <div style="width: 40vw">
    <p style="font-size: 6vw;">%.1fC / %dF<br />%.1f%%</p>
  </div>
  <div>
    <form action="/" method="post">
      <input type="text" id="goalC" name="goalc" value="%d" /><br />
      <button action="submit" name="upc" id="upC">▲</button>
      <button action="submit" name="downc" id="downC">▼</button>
    </form>
  </div>
  <form action="/" method="get" id="refresh"></form>
</body>
</html>`
