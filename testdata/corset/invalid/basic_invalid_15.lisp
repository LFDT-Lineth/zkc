;;error:5:16-19:expected bool, found u1
(defcolumns (X :i16) (BIT :binary))

(defconstraint c1 ()
  (if (== X 0) BIT (!= 0 0)))
