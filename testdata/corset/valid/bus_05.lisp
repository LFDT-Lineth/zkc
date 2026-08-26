;; Address-handoff pattern: per shard, the state module receives its entry
;; state (ADDR, VALI) and sends its exit state (ADDR, VALF).  The finalizer
;; has opposite polarity: it sends the initial state and receives the final
;; one.  Interior states cancel pairwise across shards (telescoping).
(module state)
(defcolumns (RCV :binary) (SND :binary) (ADDR :i16) (VALI :i16) (VALF :i16))
(defrecv entry bus RCV (ADDR VALI))
(defsend exit bus SND (ADDR VALF))

(module finalizer)
(defcolumns (SEL :binary) (ADDR :i16) (VAL0 :i16) (VALN :i16))
(defsend init bus SEL (ADDR VAL0))
(defrecv last bus SEL (ADDR VALN))
