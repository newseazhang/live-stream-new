import { RoomDetail } from '../../../components/room-detail';

export default async function RoomPage({ params }: { params: Promise<{ room_id: string }> }) {
  const { room_id } = await params;
  return <RoomDetail roomID={room_id} />;
}
