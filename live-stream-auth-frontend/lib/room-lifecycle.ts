type RoomOwner = { stop: () => void };
type RoomWindow = Window & { __liveStreamRoomOwner?: RoomOwner; __liveStreamRoomSuspended?: boolean };

// 当前产品每个页面只有一个房间面板。新面板接管前必须释放旧面板的资源。
export function ownRoomConnection(stop: () => void): () => void {
  const page = window as RoomWindow;
  page.__liveStreamRoomOwner?.stop();
  const owner: RoomOwner = {
    stop() {
      if (page.__liveStreamRoomOwner === owner) delete page.__liveStreamRoomOwner;
      stop();
    },
  };
  page.__liveStreamRoomOwner = owner;
  return owner.stop;
}

export function leaveCurrentRoom() {
  const page = window as RoomWindow;
  page.__liveStreamRoomSuspended = true;
  page.__liveStreamRoomOwner?.stop();
}

export function resumeRoomPage() {
  (window as RoomWindow).__liveStreamRoomSuspended = false;
}

export function isRoomPageSuspended() {
  return (window as RoomWindow).__liveStreamRoomSuspended === true;
}
