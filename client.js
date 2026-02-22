// let nickname = "prifio_k"
// let nickname = "horse"
let nickname = "chuzezemets"
// let nickname = "late_man"

let last_ind = 0
let processed_ind = 0

let serverNotifyInd       = 0
let serverSendMessageInd  = 1
let clientAskMessageInd   = 2
let clientSendMessageInd  = 3
let clientLoginInd        = 4

function req_to_json(req, ind) {
    let obj = {
        "TypeInd": ind,
        "Content": req,
    }
    return JSON.stringify(obj)
}

function add_div(txt) {
    let dv = document.createElement("div");
    dv.textContent = txt
    document.body.appendChild(dv)
}

let ws = new WebSocket("ws://localhost:8080/ws")
ws.addEventListener("open", (event) => {
    let req = {
        "Uname": nickname,
    }
    ws.send(req_to_json(req, clientLoginInd))
})

ws.onmessage = function (e) {
    let dt = e.data;
    let obj = JSON.parse(dt)
    let content = obj.Content
    console.log("Msg:", dt)
    if (obj.TypeInd == serverNotifyInd) {
        last_ind = Math.max(last_ind, content.HistoryLen)
        if (content.FirstAvailable > processed_ind) {
            add_div("* Skip " + (content.FirstAvailable - processed_ind).toString() + " messages *")
            processed_ind = content.FirstAvailable
        }
        send_reqs_loop()
    } else if (obj.TypeInd == serverSendMessageInd) {
        let res = content.Uname + ": " + content.Txt
        if (content.RequestedInd < content.ResultInd) {
            add_div("* Skip " + (content.ResultInd - content.RequestedInd).toString() + " messages *")
            processed_ind = Math.max(processed_ind, content.ResultInd + 1)
        }
        add_div(res)
        last_ind = Math.max(last_ind, content.HistoryLen)
        send_reqs_loop()
    } else {
        console.log("Invalid request type ind", dt)
    }
}

function send_reqs_loop() {
    while (processed_ind < last_ind) {
        let req = {
            "Ind": processed_ind
        }
        ws.send(req_to_json(req, clientAskMessageInd))
        processed_ind += 1
    }
}
setInterval(send_reqs_loop, 100);

function send_message(msg) {
    let req = {
        "Txt": msg
    }
    ws.send(req_to_json(req, clientSendMessageInd))
}
