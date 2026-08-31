(defcolumns (send :binary) (recv :binary) (data :i16))
;; Cannot simultaneously send/receive
(defconstraint xor () (== 0 (* send recv)))
;; Send data item when send line is high
(defsend s1 bus send (data))
;; Receive data item when recv line is high
(defrecv r1 bus recv (data))
