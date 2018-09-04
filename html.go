package main

var page = `
<html>
	<script>
		window.setInterval(function(){
			  let t = new Date().getTime()
				document.getElementById('m').src = "/sigmadelta/m?random=" + t;
				document.getElementById('o').src = "/sigmadelta/o?random=" + t;
				document.getElementById('v').src = "/sigmadelta/v?random=" + t;
				document.getElementById('e').src = "/sigmadelta/e?random=" + t;
				document.getElementById('eblur').src = "/sigmadelta/eblur?random=" + t;
		  }, 1000);
	</script>
	<body>
		<div>
			<img id="stream" display="flex" src="/stream"  style="max-width: 24%; height: auto; "/>
			<img id="m" display="flex" src="/sigmadelta/m" style="max-width: 24%; height: auto; "/>
			<img id="o" display="flex" src="/sigmadelta/o" style="max-width: 24%; height: auto; "/>
			<img id="v" display="flex" src="/sigmadelta/v" style="max-width: 24%; height: auto; "/>
			<img id="e" display="flex" src="/sigmadelta/e" style="max-width: 24%; height: auto; "/>
		</div>
	</body>
</html>
`
