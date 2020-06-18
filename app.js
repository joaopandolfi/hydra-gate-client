console.log(" === Hydra Gate Client === ")

console.log("[+] Importing libs ")

const io = require("socket.io-client")
const axios = require('axios');
const uuid = require('uuid')


var Configs = {
    ID: uuid.v1(),
    server: "http://localhost:8888",
    token:"",
    predictUrl: "http://localhost:8888/x"
}

console.log(`[+] Server ID: ${Configs.ID}`)
console.log(`[+] Connecting on server -> ${Configs.server}`)

const socket = io.connect(Configs.server)

socket.on('connect',()=>{
    console.log(`[+] Connected`)
})

socket.on('welcome',(data)=>{
    if( data == undefined)
        return console.log(`[x] Invalid welcome message`)    
    
    console.log(`[+] Welcome message -> ${data.msg}`)
    console.log(`[-] Authenticating`)
    socket.emit('register',{token: Configs.token, id: Configs.ID})
})

socket.on("registered",(data)=>{
    console.log(`[+] Authenticated -> ${data.sid}`)
})

// @receives {id:uuid,data:any, timestamp:Date}
socket.on("predict",(payload)=>{
    console.log(`[+] <== pred: id [${payload.id}] timestamp [${payload.timestamp}]`)
    axios({
        method:"POST",
        url:Configs.predictUrl,
        data:payload.data
      })
    .then(result =>{
        console.log(`[+] ==> pred: id [${payload.id}] timestamp [${(new Date()).getTime()}]`)
        socket.emit('predicted',{id:payload.id,success:true,data:result.data})
    } )
    .catch(err => {
        console.log(err)
        console.log(`[-] x=x Error on predict: id [${payload.id}] timestamp [${(new Date()).getTime()}] error [${err}]`)
        socket.emit('predicted',{id:payload.id,success:false,data:{}})
  })
})

socket.on('disconnect',()=>{
    console.log(`[-] Disconnected`)
})
