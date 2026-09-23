import { BookingChat } from "@/components/booking-chat";

export default async function Page({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  return <BookingChat key={slug} slug={slug} />;
}
