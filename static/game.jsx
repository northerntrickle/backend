/** @jsx React.DOM */

var ENABLE_PHYSICS = false;
var BASE_URL = "10.21.55.189:3000";

var NORTH = 0;
var EAST = 1;
var SOUTH = 2;
var WEST = 3;

var EVENT_PLAYER_MOVE = 0;
var EVENT_PLAYER_ATTACK = 1;
var EVENT_PLAYER_DAMAGE = 2;
var EVENT_CHAT = 3;

var net = {};


function constructWsURI() {
  var loc = window.location, new_uri;
  if (loc.protocol === "https:") {
    new_uri = "wss:";
  } else {
    new_uri = "ws:";
  }
  new_uri += "//" + loc.host;
  new_uri += loc.pathname + "connect";
  return new_uri;
}

var login = function(username, password, cb){
  var http = new XMLHttpRequest();
  var httpRegister = new XMLHttpRequest();

  var urlRegister = "http://"+BASE_URL+"/sign_up";
  httpRegister.open("POST", urlRegister, true);
  httpRegister.onreadystatechange = function(){
    var url = "http://"+BASE_URL+"/login";
    http.open("POST", url, true);
    //Send the proper header information along with the request
    http.setRequestHeader("Content-Type", "application/json");
    //http.setRequestHeader("Content-length", params.length);
    //http.setRequestHeader("Connection", "close");

    http.onreadystatechange = function() {//Call a function when the state changes.
      if(http.readyState == 4 && http.status == 201) {
        cb(JSON.parse(http.responseText));
      }
    }
    http.send(JSON.stringify({
      username: username,
      password: password
    }));
  }
  httpRegister.send(JSON.stringify({
    username: username,
    password: password
  }));
}



var Network = (function NetworkClosure(){
  var Network = function Network_constructor(auth){
    this.auth = auth;
    this.ws = new WebSocket("ws://"+BASE_URL+"/connect?auth="+auth);
    this.eventHandlers = [];
  }
  Network.prototype = {
    initialize: function(){
      var self = this;
      net = this;
      this.ws.onmessage = this.handleMessage(this);
      this.ws.onclose = function reconnect (){
        setTimeout(function(){
          console.log("reconnected");
          try {
          self.ws = new WebSocket("ws://"+BASE_URL+"/connect?auth="+self.auth);
          } catch (e){
            reconnect();
          }
        }, 200);
      }.bind(this);
    },
    send: function(payload){
      this.ws.send(payload);
    },
    handleMessage: function(that){
      return function(e){
        var data = JSON.parse(e.data);
        that.eventHandlers.map(function(handler){
          if(handler[0] == data["type"]){
            handler[1](data);
          }
        });
      }
    },
    on: function(eventName, callback){
      this.eventHandlers.push([eventName, callback]);
    }
  }
  return Network;
})();


var Player = (function PlayerClosure(){
  var Player = function Player_constructor(game, options){
    var options = options || {};
    this.health = 100;
    this.inventory = [];
    this.sprite = game.add.tileSprite(options.x || window.screen.width/2, options.y || window.screen.height/2, 13, 16, 'tilesBoys');
    //this.sprite.animations.add('ani', [0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15], 1, true);
    this.sprite.animations.add('N', [4]);
    this.sprite.animations.add('E', [0]);
    this.sprite.animations.add('S', [6]);
    this.sprite.animations.add('W', [2]);

    this.sprite.animations.add('AE', [1]);
    this.sprite.animations.add('AW', [3]);
    this.sprite.animations.add('AN', [5]);
    this.sprite.animations.add('AS', [7]);

    if(ENABLE_PHYSICS){
      game.physics.p2.enable(this.sprite);
      this.sprite.body.setCircle(10);
      this.sprite.body.kinematic = true;
    }
  }

  Player.prototype = {

    update: function Player_update(){
      this.sprite.body.setZeroVelocity();
    },

    setPos: function Player_setPos(x,y){
      this.sprite.x = x;
      this.sprite.y = y;
    },

  }

  return Player;
})();


var Weapon = (function WeaponClosure(){
  var Weapon = function Weapon_constructor(game, options){
    var options = options || {};
    this.sprite = game.add.sprite(options.x || 0, options.y || 0, 'tilesBoys');

    if(ENABLE_PHYSICS){
      game.physics.p2.enable(this.sprite);
      this.sprite.body.setCircle(10);
      this.sprite.body.kinematic = true;
    }
  }

  Weapon.prototype = {

    update: function Weapon_update(){
    },

  }

  return Weapon;
})();


var InputActor = (function InputActorClosure(){
  var InputActor = function InputActor_constructor(net, playerPawn){
    var self = this;
    this.net = net;
    this.player = playerPawn;
    this.playerHealth = 6;
    this.facing = 0;
  }

  InputActor.prototype = {
    move: function InputActor_move (direction){
      this.net.send(JSON.stringify({
        type: EVENT_PLAYER_MOVE,
        body: { direction: direction }
      }));
      switch(direction){
        case NORTH: 
          this.player.sprite.y -= 1; 
          this.player.sprite.animations.play('N', 20, true);
          break;
        case EAST: 
          this.player.sprite.x += 1; 
          this.player.sprite.animations.play('E', 20, true);
          break;
        case SOUTH: 
          this.player.sprite.y += 1; 
          this.player.sprite.animations.play('S', 20, true);
          break;
        case WEST: 
          this.player.sprite.x -= 1; 
          this.player.sprite.animations.play('W', 20, true);
          break;
      }
      this.facing = direction;
    },

    attack: function InputActor_attack(){
      this.net.send(JSON.stringify({
        type: EVENT_PLAYER_ATTACK,
      }));
      switch(this.facing){
        case NORTH: 
          this.player.sprite.animations.play('AN', 20, true);
          break;
        case EAST: 
          this.player.sprite.animations.play('AE', 20, true);
          break;
        case SOUTH: 
          this.player.sprite.animations.play('AS', 20, true);
          break;
        case WEST: 
          this.player.sprite.animations.play('AW', 20, true);
          break;
      }
    }
  }

  return InputActor;
})();


var Hortons = (function HortonsClosure(){
  var Hortons = function Hortons_constructor(){
    this.health = 12;
    this.sprite = game.add.sprite(200, 200, 'hortons');
  }

  Hortons.prototype = {

  }

  return Hortons;
})();


var Game = (function GameClosure(){

  var Game = function Game_constructor(){
    this.pawns = [];
  }

  Game.prototype = {
    init: function(userId, auth){
      this.net = new Network(auth);
      this.userId = userId;
      this.net.initialize();
      this.players = {};
      this.game = new Phaser.Game(200, 150, Phaser.CANVAS, 'trickle', {
        preload: this.preload(this),
        create: this.create(this),
        update: this.update(this),
        render: this.render(this)}, false, false)
    },

    render: function Game_render(that){
      return function(){
        that.context.drawImage(this.game.canvas, 0, 0, this.game.width, this.game.height, 0, 0, that.width, that.height);
      }
    },

    preload: function Game_preload(that){
      return function(){
        this.game.load.tilemap('hortons', 'static/assets/map.json', null, Phaser.Tilemap.TILED_JSON);
        this.game.load.image('tilesGrass', 'static/assets/Assets/grass.png');
        this.game.load.image('tilesRoad', 'static/assets/Assets/road.png');
        this.game.load.image('tilesPlants', 'static/assets/Assets/plats.png');
        this.game.load.image('tilesFlags', 'static/assets/Assets/flags.png');
        this.game.load.image('tilesBuildings', 'static/assets/Assets/building.png');
        this.game.load.image('tilesGirls', 'static/assets/Assets/girl.png');
        this.game.load.spritesheet('tilesBoys', 'static/assets/Assets/boy.png', 16, 16);
        this.game.load.image('tilesFurniture', 'static/assets/Assets/furniture.png');
        this.game.load.image('tilesTims', 'static/assets/Assets/tim.png');
      }
    },

    create: function Game_create(that){
      return function(){
        //that.game.add.image(0, 0, 'sky');
        //,phaser/build/
        that.game.canvas.style['display'] = 'none';

        that.canvas = Phaser.Canvas.create(that.game.width * 4, that.game.height * 4);

        //  Store a reference to the Canvas Context
        that.context = that.canvas.getContext('2d');

        //  Add the scaled canvas to the DOM
        Phaser.Canvas.addToDOM(that.canvas);

        //  Disable smoothing on the scaled canvas
        Phaser.Canvas.setSmoothingEnabled(that.context, false);

        //  Cache the width/height to avoid looking it up every render
        that.width = that.canvas.width;
        that.height = that.canvas.height;

        that.game.stage.smooth = false;
        that.game.world.setBounds(-200, -150, 400, 300);
        //that.game.world.scale.setTo(2, 2);
        var map = that.game.add.tilemap('hortons');
        map.addTilesetImage('Grass', 'tilesGrass');
        map.addTilesetImage('Road', 'tilesRoad');
        map.addTilesetImage('Plants', 'tilesPlants', 14, 16);
        map.addTilesetImage('building', 'tilesBuildings');
        map.addTilesetImage('flags', 'tilesFlags', 14, 32);
        map.addTilesetImage('girl', 'tilesGirls', 13, 16);
        map.addTilesetImage('boy', 'tilesBoys', 13, 16);
        map.addTilesetImage('tim', 'tilesTims');
        var layerGrass = map.createLayer('Grass');
        layerGrass.resizeWorld();
        layerGrass.wrap = true;
        var layerRoad = map.createLayer('RoadPlants');
        layerRoad.resizeWorld();
        layerRoad.wrap = true;
        var layerLabels = map.createLayer('Label');
        layerLabels.resizeWorld();
        layerLabels.wrap = true;
        var layerPeople = map.createLayer('PEOPLE');
        layerPeople.resizeWorld();
        layerPeople.wrap = true;

        that.players[that.userId] = new Player(that.game);


        that.game.camera.follow(that.players[that.userId].sprite, Phaser.Camera.FOLLOW_TOPDOWN);


        that.input = new InputActor(that.net, that.players[that.userId] )

        //that.players[that.userId].name = index.toString();
       //#that.game.physics.enable(that.players[that.userId].sprite, Phaser.Physics.ARCADE);
        //hat.players[that.userId].sprite.body.immovable = false;
        //that.players[that.userId].sprite.body.collideWorldBounds = true;
        //that.players[that.userId].sprite.body.bounce.setTo(1, 1);

        that.net.on(EVENT_PLAYER_MOVE, function(data){
          if(that.players[data.user_id]){
            that.players[data.user_id].sprite.x = data.body.dimensions.x;
            that.players[data.user_id].sprite.y = data.body.dimensions.y;
          }else{
            that.players[data.user_id] = new Player(that.game, {
              x: data.body.dimensions.x,
              y: data.body.dimensions.y
            })
          }
        });


        that.net.on(EVENT_PLAYER_DAMAGE, function(data){
          if(that.players[data.user_id]){
            that.players[data.user_id].sprite.x = data.body.dimensions.x;
            that.players[data.user_id].sprite.y = data.body.dimensions.y;
          }else{
            that.players[data.user_id] = new Player(that.game, {
              x: data.body.dimensions.x,
              y: data.body.dimensions.y
            })
          }
        });

        //this.pawns.push();
        // TODO: spawn Hortons


        if(ENABLE_PHYSICS){
          that.game.physics.startSystem(Phaser.Physics.P2JS); //	Enable p2 physics
          that.game.physics.p2.restitution = 0.2; //  Bouncyness
          that.game.physics.p2.enable( [ kinematic1, kinematic2 ]);
          // Enable if for physics. This creates a default rectangular body.
        }

        text = that.game.add.text(20, 20, 'move with arrow keys', { fill: '#ffffff' });

        that.cursors = that.game.input.keyboard.createCursorKeys();
        that.fireButton = that.game.input.keyboard.addKey(Phaser.Keyboard.TILDE);
        that.fireButton.onDown.add(that.input.attack);
      }
    },

    update: function Game_update(that){
      return function(){
        var cursors = that.cursors;
        if (cursors.left.isDown) {
          that.input.move(WEST);
        } else if (cursors.right.isDown) {
          that.input.move(EAST);
        }
        if (cursors.up.isDown) {
            that.input.move(NORTH);
        } else if (cursors.down.isDown) {
            that.input.move(SOUTH);
        }
        
      }
    },
  }
  return Game;
})()


var Chat = React.createClass({
  getInitialState: function(){
    return { messages: []}
  },
  componentWillMount: function(){
    var self = this;
    net.on(EVENT_CHAT, function(data){
      self.state.messages.push(data.body.msg);
      self.setState({messages: self.state.messages});
      self.forceUpdate();
    });
  },
  handleChange: function(e){
    this.setState({newMessage: e.target.value});
  },
  sendMessage: function(e){
    var self = this;
    net.send(JSON.stringify({
      type: EVENT_CHAT,
      body: {msg: self.state.newMessage}
    }));
    this.setState({newMessage: ""});
    return false;
  },
  render: function(){
    var messages = this.state.messages.map(function(message){
      return (<p>{message}</p>)
    });
    return (<div className="chat">
            {messages}
            <form onSubmit={this.sendMessage}>
              <input type="text" value={this.state.newMessage} onChange={this.handleChange}/>
              <button type="button" onClick={this.sendMessage}>Send</button>
            </form>
            </div>)
  }
});

 


var Login = React.createClass({
  getInitialState: function(){
    return { login: "", pass: "" }
  },
  handleChangeLogin: function(event){
    this.setState({ login: event.target.value });
  },
  handleChangePass: function(e){
    this.setState({ pass: event.target.value });
  },
  handleLogin: function(){
    var self = this;
    login(this.state.login, this.state.pass, function(res){
      var game = new Game();
      game.init(res.user_id, res.token);
      self.loggedIn = true;
      self.forceUpdate();
    });
  },
  render: function(){
    if(!this.loggedIn){
      return (
        <form action="">
          <input type="text" onChange={this.handleChangeLogin} value={this.state.login} placeholder="Login"/>
          <input type="password" onChange={this.handleChangePass} value={this.state.pass} placeholder="Password"/>
          <button type="button" onClick={this.handleLogin}>Login</button>
        </form>
      )
    } else {
      return (
        <div> 
          <Chat></Chat>
          <a onClick={this.handleLogin}>Logout</a> 
        </div>
      )
    }
  }
});




var HealthBar = React.createClass({
  render: function(){
    var hearts = [];
    for(var i=0; i<this.props.count; i++){ hearts.push({}); }
    return (
      <div className="health">
        {hearts.map(function(){<div className="health-item"></div>})}
      </div>
    )
  }
});


var PlayerSidebar = React.createClass({
  getInitialState: function(){
    return { health: 6, captures: 0, loggedIn: false}
  },
  handleChange: function(){

  },
  render: function(){
    return (
      <div className="sidebar"> <Login/> </div>
    )
  }
});

React.renderComponent(<PlayerSidebar/>, document.body);


var Leaderboard = React.createClass({
  getInitialState: function(){
    return { messages: []}
  },
  render: function(){

  }
});

