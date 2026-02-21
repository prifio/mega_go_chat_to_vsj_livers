let nickname = "prifio_k"
// let nickname = "horse"
// let nickname = "chuzezemets"
// let nickname = "late_man"

let ind = 0
let processed_ind = 0

let ws = new WebSocket("ws://localhost:8080/ws")
ws.addEventListener("open", (event) => {
    ws.send(nickname)
})

ws.onmessage = function (e) {
    let dt = e.data;
    console.log("Msg:", dt)
    if (dt[0] == "0") {
        let cur_ind = parseInt(dt.substring(1, dt.length))
        ind = Math.max(ind, cur_ind)
        background_loop()
    } else if (dt[0] == "1") {
        let res = dt.substring(1, dt.length)
        console.log("Incomed:", res)
        let dv = document.createElement("div");
        dv.textContent = res
        document.body.appendChild(dv)
    } else {
        console.log("ERR_1", dt)
    }
}

function background_loop() {
    while (processed_ind < ind) {
        ws.send("0" + processed_ind.toString())
        processed_ind += 1
    }
}
setInterval(background_loop, 100);

function send_message(msg) {
    ws.send("1" + msg)
}
