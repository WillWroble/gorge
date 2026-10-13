package org.mage.test.oracle;

import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.util.ArrayList;
import java.util.List;
import mage.constants.PhaseStep;
import org.mage.test.player.TestPlayer;
import sun.misc.Unsafe;

/** Executable check of two driver rules, without H2 or a game: which seat
 * carries a step's checkpoint, and how an activate command is bound to the
 * card the scenario names. Run scripts/xmage-oracle-test-checkpoint-seat.sh.
 * The end-to-end rows (Eye of Jace, Gideon the Oathless, Found Footage,
 * Ajani Unrelenting, Surveillance Phantasm) are XMage replays. */
public final class ScenarioReplayCheckpointSeatTest {
    /** Records the registered code actions instead of queueing them. */
    public static final class RecordingDriver extends ScenarioReplay {
        List<String> codeActions = new ArrayList<>();

        @Override
        public void runCode(String info, int turnNum, PhaseStep step, TestPlayer player,
                org.mage.test.serverside.base.CardTestCodePayload payload) {
            codeActions.add(info);
        }
    }

    private static void equal(Object want, Object got) {
        if (!java.util.Objects.equals(want, got)) {
            throw new AssertionError("expected " + want + ", got " + got);
        }
    }

    private static void set(ScenarioReplay d, String name, Object value) throws Exception {
        // playerA/playerB live on the framework superclass.
        for (Class<?> c = ScenarioReplay.class; c != null; c = c.getSuperclass()) {
            try {
                Field f = c.getDeclaredField(name);
                f.setAccessible(true);
                f.set(d, value);
                return;
            } catch (NoSuchFieldException next) {
                // keep climbing
            }
        }
        throw new NoSuchFieldException(name);
    }

    public static void main(String[] args) throws Exception {
        Field uf = Unsafe.class.getDeclaredField("theUnsafe");
        uf.setAccessible(true);
        Unsafe unsafe = (Unsafe) uf.get(null);
        RecordingDriver d = (RecordingDriver) unsafe.allocateInstance(RecordingDriver.class);
        d.codeActions = new ArrayList<>();
        TestPlayer a = (TestPlayer) unsafe.allocateInstance(TestPlayer.class);
        TestPlayer b = (TestPlayer) unsafe.allocateInstance(TestPlayer.class);
        set(d, "playerA", a);
        set(d, "playerB", b);
        set(d, "turn", 2);
        set(d, "phase", PhaseStep.PRECOMBAT_MAIN);
        java.util.Map<String, String> alias = new java.util.LinkedHashMap<>();
        alias.put("p0:Ajani Goldmane", "@p0:Ajani Goldmane");
        set(d, "refAlias", alias);

        // Preconditions: the two seats are different objects.
        equal(false, a == b);

        Method seatFor = ScenarioReplay.class.getDeclaredMethod("checkpointSeat", String.class, JsonObject.class);
        seatFor.setAccessible(true);
        JsonObject seat0 = JsonParser.parseString("{\"op\":\"pass_to\",\"seat\":0}").getAsJsonObject();
        JsonObject seat1Resolve = JsonParser.parseString("{\"op\":\"resolve\",\"seat\":1}").getAsJsonObject();
        JsonObject seat0Cast = JsonParser.parseString("{\"op\":\"cast\",\"seat\":1}").getAsJsonObject();

        // pass_to checkpoints on the active seat, whatever seat the step names.
        set(d, "activeSeat", 1);
        equal(true, seatFor.invoke(d, "pass_to", seat0) == b);
        set(d, "activeSeat", 0);
        equal(true, seatFor.invoke(d, "pass_to", seat0) == a);
        // A seat-1 resolve waits on seat 1's queue; other ops keep seat 0.
        equal(true, seatFor.invoke(d, "resolve", seat1Resolve) == b);
        equal(true, seatFor.invoke(d, "cast", seat0Cast) == a);

        // The activation text names its card: a space-free alias prefix, so
        // "+1" no longer matches whichever planeswalker XMage lists first.
        Method bound = ScenarioReplay.class.getDeclaredMethod("boundActivationText",
                TestPlayer.class, String.class, String.class, int.class);
        bound.setAccessible(true);
        equal("@act1 +1", bound.invoke(d, a, "p0:Ajani Goldmane", "+1", 1));
        equal(List.of("bind p0:Ajani Goldmane"), d.codeActions);
        // A ref with no setup alias keeps the plain text and binds nothing.
        equal("+1", bound.invoke(d, a, "p0:Ajani Unrelenting", "+1", 2));
        equal(1, d.codeActions.size());
        System.out.println("PASS checkpoint seat and bound activation text");
    }
}
